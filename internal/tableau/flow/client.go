package flow

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	defaultPageSize        = 100
	maximumPageSize        = 1000
	metadataResponseLimit  = 4 * 1024 * 1024
	defaultUploadThreshold = 64 * 1024 * 1024
	defaultUploadChunkSize = 64 * 1024 * 1024
)

// RESTClient implements the released Tableau flow REST contract.
type RESTClient struct {
	transport       *tableau.Transport
	session         auth.Session
	serverURL       string
	uploadThreshold int64
	uploadChunkSize int64
}

// NewClient creates an authenticated flow REST client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *RESTClient {
	return &RESTClient{
		transport: transport, session: session, serverURL: serverURL,
		uploadThreshold: defaultUploadThreshold, uploadChunkSize: defaultUploadChunkSize,
	}
}

// SetUploadThreshold overrides the direct-publish threshold for deterministic tests.
func (c *RESTClient) SetUploadThreshold(limit int64) {
	if c != nil && limit > 0 && limit <= defaultUploadThreshold {
		c.uploadThreshold = limit
	}
}

// SetUploadChunkSize overrides the upload block size for deterministic tests.
func (c *RESTClient) SetUploadChunkSize(limit int64) {
	if c != nil && limit > 0 && limit <= defaultUploadChunkSize {
		c.uploadChunkSize = limit
	}
}

// List returns one bounded classic REST flow page.
func (c *RESTClient) List(ctx context.Context, input ListRequest) (Page, error) {
	if err := c.validate(); err != nil {
		return Page{}, err
	}
	if input.PageNumber == 0 {
		input.PageNumber = 1
	}
	if input.PageSize == 0 {
		input.PageSize = defaultPageSize
	}
	if input.PageNumber < 1 {
		return Page{}, errors.New("flow list page number must be positive")
	}
	if input.PageSize < 1 || input.PageSize > maximumPageSize {
		return Page{}, fmt.Errorf("flow list page size must be between 1 and %d", maximumPageSize)
	}
	query := url.Values{
		"pageNumber": {strconv.Itoa(input.PageNumber)},
		"pageSize":   {strconv.Itoa(input.PageSize)},
	}
	filter, err := ListFilter(input)
	if err != nil {
		return Page{}, err
	}
	if filter != "" {
		query.Set("filter", filter)
	}
	response, err := c.do(ctx, http.MethodGet, c.sitePath("flows"), query, nil, "", "flow.list", metadataResponseLimit)
	if err != nil {
		return Page{}, err
	}
	var envelope listEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Page{}, tableau.NewProtocolError("flow.list", response, fmt.Errorf("decode flow list response: %w", err), true)
	}
	page, err := normalizePage(envelope.Pagination, input.PageNumber, input.PageSize, len(envelope.Flows))
	if err != nil {
		return Page{}, tableau.NewProtocolError("flow.list", response, err, true)
	}
	page.Items = make([]Flow, len(envelope.Flows))
	for index, item := range envelope.Flows {
		page.Items[index] = normalizeFlow(item, nil)
	}
	page.TableauRequestID = response.TableauRequestID
	return page, nil
}

// Get returns one exact flow by authoritative LUID.
func (c *RESTClient) Get(ctx context.Context, flowLUID string) (Flow, error) {
	if strings.TrimSpace(flowLUID) == "" {
		return Flow{}, errors.New("flow LUID is required")
	}
	if err := c.validate(); err != nil {
		return Flow{}, err
	}
	response, err := c.do(ctx, http.MethodGet, c.sitePath("flows", flowLUID), nil, nil, "", "flow.get", metadataResponseLimit)
	if err != nil {
		return Flow{}, err
	}
	var envelope getEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Flow{}, tableau.NewProtocolError("flow.get", response, fmt.Errorf("decode flow response: %w", err), true)
	}
	result := normalizeFlow(envelope.Flow, envelope.OutputSteps)
	result.TableauRequestID = response.TableauRequestID
	if result.LUID != flowLUID {
		return Flow{}, tableau.NewProtocolError("flow.get", response, fmt.Errorf("flow response returned LUID %q, expected %q", result.LUID, flowLUID), true)
	}
	return result, nil
}

// Download streams the native TFL or TFLX bytes unchanged into destination
// and returns the filename. On error destination may hold a partial body.
func (c *RESTClient) Download(ctx context.Context, flowLUID string, destination io.Writer) (Download, error) {
	if strings.TrimSpace(flowLUID) == "" {
		return Download{}, errors.New("flow LUID is required")
	}
	if err := c.validate(); err != nil {
		return Download{}, err
	}
	var filename string
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodGet, ServerURL: c.serverURL, Path: c.sitePath("flows", flowLUID, "content"),
		Accept: "application/xml", Operation: "flow.pull", OpenResponseBody: func(response tableau.Response) (io.Writer, error) {
			var err error
			filename, err = tableau.DownloadFilename("flow.pull", "flow", response, ".tfl", ".tflx")
			return destination, err
		},
	})
	if err != nil {
		return Download{}, err
	}
	return Download{Filename: filename, TableauRequestID: response.TableauRequestID}, nil
}

// Prepare snapshots and, when needed, uploads native flow bytes without final publication.
func (c *RESTClient) Prepare(ctx context.Context, input PublishRequest) (PreparedPublish, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	extension, err := validatePublishRequest(input)
	if err != nil {
		return nil, err
	}
	snapshot, err := tableau.SnapshotPublishPayload(ctx, input.ContentPath, input.ContentSize, input.ExpectedFingerprint)
	if err != nil {
		return nil, fmt.Errorf("prepare flow publish payload: %w", err)
	}
	prepared := false
	defer func() {
		if !prepared {
			_ = snapshot.Close()
		}
	}()
	payload, err := publishPayload(input.Name, input.ProjectLUID)
	if err != nil {
		return nil, err
	}
	parts := []tableau.MultipartPart{tableau.RequestPayloadPart(payload)}
	query := url.Values{"overwrite": {strconv.FormatBool(input.Overwrite)}}
	if snapshot.Size() > c.uploadThreshold {
		upload := tableau.FileUpload{Transport: c.transport, Session: c.session, ServerURL: c.serverURL, Operation: "flow", Filename: input.Filename, ChunkSize: c.uploadChunkSize}
		uploadSessionID, err := upload.Upload(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		query.Set("uploadSessionId", uploadSessionID)
		query.Set("flowType", extension)
	} else {
		parts = append(parts, tableau.FilePart("tableau_flow", input.Filename, snapshot.File(), 0, snapshot.Size()))
	}
	body, err := tableau.NewMultipartBody(parts...)
	if err != nil {
		return nil, err
	}
	prepared = true
	return &preparedPublish{
		client: c, query: query, body: body, snapshot: snapshot,
		expectedName: input.Name, expectedProjectLUID: input.ProjectLUID,
	}, nil
}

// Move moves one exact flow to one project on the current site.
func (c *RESTClient) Move(ctx context.Context, flowLUID, projectLUID string) (MutationResult, error) {
	if strings.TrimSpace(flowLUID) == "" || strings.TrimSpace(projectLUID) == "" {
		return MutationResult{}, errors.New("flow move requires exact flow and project LUIDs")
	}
	if err := c.validate(); err != nil {
		return MutationResult{}, err
	}
	body, err := moveBody(projectLUID)
	if err != nil {
		return MutationResult{}, err
	}
	response, err := c.do(ctx, http.MethodPut, c.sitePath("flows", flowLUID), nil, body, "application/xml", "flow.move", metadataResponseLimit)
	if err != nil {
		return uncertainMutation(flowLUID, projectLUID, err)
	}
	if response.StatusCode != http.StatusOK {
		result := MutationResult{Status: "unknown", FlowLUID: flowLUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.move", response, fmt.Errorf("flow move returned HTTP %d, expected 200", response.StatusCode), false)
	}
	var envelope getEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		result := MutationResult{Status: "unknown", FlowLUID: flowLUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.move", response, fmt.Errorf("decode flow move response: %w", err), false)
	}
	if envelope.Flow.ID != flowLUID || envelope.Flow.Project.ID != projectLUID {
		result := MutationResult{Status: "unknown", FlowLUID: flowLUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.move", response, fmt.Errorf("flow move response returned flow %q in project %q, expected flow %q in project %q", envelope.Flow.ID, envelope.Flow.Project.ID, flowLUID, projectLUID), false)
	}
	return MutationResult{Status: "succeeded", FlowLUID: flowLUID, ProjectLUID: projectLUID, TableauRequestID: response.TableauRequestID}, nil
}

// Update changes the owner of one exact flow.
// Tableau's Update Flow endpoint does not support changing the flow name.
func (c *RESTClient) Update(ctx context.Context, input UpdateRequest) (MutationResult, error) {
	input.LUID = strings.TrimSpace(input.LUID)
	if input.LUID == "" || input.OwnerLUID == nil || strings.TrimSpace(*input.OwnerLUID) == "" {
		return MutationResult{}, errors.New("flow update requires exact flow and owner LUIDs")
	}
	if err := c.validate(); err != nil {
		return MutationResult{}, err
	}
	ownerLUID := strings.TrimSpace(*input.OwnerLUID)
	response, err := c.do(ctx, http.MethodPut, c.sitePath("flows", input.LUID, "owner", ownerLUID), nil, nil, "", "flow.update", metadataResponseLimit)
	if err != nil {
		return uncertainMutation(input.LUID, "", err)
	}
	if response.StatusCode != http.StatusOK {
		result := MutationResult{Status: "unknown", FlowLUID: input.LUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.update", response, fmt.Errorf("flow update returned HTTP %d, expected 200", response.StatusCode), false)
	}
	if len(response.Body) != 0 {
		result := MutationResult{Status: "unknown", FlowLUID: input.LUID, OwnerLUID: ownerLUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.update", response, fmt.Errorf("flow owner update returned %d response bytes, expected an empty response", len(response.Body)), false)
	}
	return MutationResult{Status: "succeeded", FlowLUID: input.LUID, OwnerLUID: ownerLUID, TableauRequestID: response.TableauRequestID}, nil
}

// Delete deletes one exact flow and accepts only Tableau's documented 204 response.
func (c *RESTClient) Delete(ctx context.Context, flowLUID string) (MutationResult, error) {
	if strings.TrimSpace(flowLUID) == "" {
		return MutationResult{}, errors.New("flow delete requires an exact flow LUID")
	}
	if err := c.validate(); err != nil {
		return MutationResult{}, err
	}
	response, err := c.do(ctx, http.MethodDelete, c.sitePath("flows", flowLUID), nil, nil, "", "flow.delete", metadataResponseLimit)
	if err != nil {
		return uncertainMutation(flowLUID, "", err)
	}
	if response.StatusCode != http.StatusNoContent || len(response.Body) != 0 {
		result := MutationResult{Status: "unknown", FlowLUID: flowLUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.delete", response, fmt.Errorf("flow delete returned HTTP %d with %d response bytes, expected empty HTTP 204", response.StatusCode, len(response.Body)), false)
	}
	return MutationResult{Status: "succeeded", FlowLUID: flowLUID, TableauRequestID: response.TableauRequestID}, nil
}

type preparedPublish struct {
	client              *RESTClient
	query               url.Values
	body                *tableau.MultipartBody
	snapshot            *tableau.PublishSnapshot
	expectedName        string
	expectedProjectLUID string
}

// Commit sends the final publish request exactly once, then releases the publish
// snapshot. A snapshot cleanup failure never changes the publish result.
func (p *preparedPublish) Commit(ctx context.Context) (PublishResult, error) {
	if p == nil || p.client == nil {
		return PublishResult{}, errors.New("flow publish is not prepared")
	}
	if err := p.snapshot.Claim(); err != nil {
		return PublishResult{}, fmt.Errorf("prepared flow publish: %w", err)
	}
	defer func() { _ = p.snapshot.Close() }()

	response, err := p.client.doMultipart(ctx, http.MethodPost, p.client.sitePath("flows"), p.query, p.body, "flow.publish")
	if err != nil {
		if isUncertain(err) {
			return PublishResult{Status: "unknown", TableauRequestID: tableau.RequestID(err)}, err
		}
		return PublishResult{}, err
	}
	if response.StatusCode != http.StatusCreated {
		result := PublishResult{Status: "unknown", TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.publish", response, fmt.Errorf("flow publish returned HTTP %d, expected 201", response.StatusCode), false)
	}
	var envelope getEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		result := PublishResult{Status: "unknown", FlowLUID: envelope.Flow.ID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.publish", response, fmt.Errorf("decode flow publish response: %w", err), false)
	}
	if envelope.Flow.ID == "" || envelope.Flow.Name != p.expectedName || envelope.Flow.Project.ID != p.expectedProjectLUID {
		result := PublishResult{Status: "unknown", FlowLUID: envelope.Flow.ID, FlowName: envelope.Flow.Name, ProjectLUID: envelope.Flow.Project.ID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("flow.publish", response, fmt.Errorf("flow publish response returned flow %q named %q in project %q", envelope.Flow.ID, envelope.Flow.Name, envelope.Flow.Project.ID), false)
	}
	return PublishResult{Status: "succeeded", FlowLUID: envelope.Flow.ID, FlowName: envelope.Flow.Name, ProjectLUID: envelope.Flow.Project.ID, TableauRequestID: response.TableauRequestID}, nil
}

func (c *RESTClient) do(ctx context.Context, method, path string, query url.Values, body []byte, contentType, operation string, responseLimit int64) (tableau.Response, error) {
	if err := c.validate(); err != nil {
		return tableau.Response{}, err
	}
	return c.transport.Do(ctx, c.session, tableau.Request{
		Method: method, ServerURL: c.serverURL, Path: path, Query: query, Body: body,
		ContentType: contentType, Accept: "application/xml", Operation: operation, MaxResponseBytes: responseLimit,
	})
}

func (c *RESTClient) doMultipart(ctx context.Context, method, path string, query url.Values, body *tableau.MultipartBody, operation string) (tableau.Response, error) {
	if err := c.validate(); err != nil {
		return tableau.Response{}, err
	}
	return c.transport.Do(ctx, c.session, tableau.Request{
		Method: method, ServerURL: c.serverURL, Path: path, Query: query, Multipart: body,
		Accept: "application/xml", Operation: operation, MaxResponseBytes: metadataResponseLimit,
	})
}

func (c *RESTClient) validate() error {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return errors.New("authenticated flow client is not configured")
	}
	if strings.TrimSpace(c.session.SiteLUID()) == "" {
		return errors.New("authenticated flow session omitted the site LUID")
	}
	return nil
}

func (c *RESTClient) sitePath(parts ...string) string {
	segments := []string{"api", c.transport.APIVersion(), "sites", c.session.SiteLUID()}
	segments = append(segments, parts...)
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return "/" + strings.Join(segments, "/")
}

// ListFilter validates and encodes flow selectors for paged and full lists.
// Pagination fields do not affect the selected population.
func ListFilter(input ListRequest) (string, error) {
	fields := []struct{ name, value string }{
		{name: "name", value: input.Name},
		{name: "ownerName", value: input.OwnerName},
		{name: "projectId", value: input.ProjectLUID},
		{name: "projectName", value: input.ProjectName},
	}
	filters := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		if strings.ContainsAny(field.value, "&,") {
			return "", fmt.Errorf("flow filter %s cannot contain ampersand or comma", field.name)
		}
		filters = append(filters, field.name+":eq:"+field.value)
	}
	return strings.Join(filters, ","), nil
}

func normalizePage(input paginationXML, expectedNumber, maximumSize, count int) (Page, error) {
	if input.Number == nil || input.Size == nil || input.Total == nil {
		return Page{}, errors.New("flow list response omitted pagination")
	}
	if *input.Number != expectedNumber || *input.Size < 1 || *input.Size > maximumSize || *input.Total < 0 || count > *input.Size || (*input.Number-1)*(*input.Size)+count > *input.Total {
		return Page{}, errors.New("flow list returned inconsistent pagination")
	}
	return Page{Number: *input.Number, Size: *input.Size, Total: *input.Total}, nil
}

func normalizeFlow(input flowXML, output []outputStepXML) Flow {
	tags := make([]string, len(input.Tags))
	for index, tag := range input.Tags {
		tags[index] = tag.Label
	}
	parameters := make([]Parameter, len(input.Parameters))
	for index, parameter := range input.Parameters {
		parameters[index] = Parameter{LUID: parameter.ID, Name: parameter.Name, Type: parameter.Type, Description: parameter.Description, Value: parameter.Value, Required: parameter.Required}
	}
	steps := make([]OutputStep, len(output))
	for index, step := range output {
		steps[index] = OutputStep{LUID: step.ID, Name: step.Name}
	}
	return Flow{
		LUID: input.ID, Name: input.Name, Description: input.Description, FileType: input.FileType,
		ProjectLUID: input.Project.ID, ProjectName: input.Project.Name, OwnerLUID: input.Owner.ID,
		CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt, Tags: tags, Parameters: parameters, OutputSteps: steps,
	}
}

func validatePublishRequest(input PublishRequest) (string, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.ProjectLUID) == "" || strings.TrimSpace(input.Filename) == "" {
		return "", errors.New("flow publish requires name, project LUID, and filename")
	}
	if !tableau.ValidFilename(input.Filename) {
		return "", fmt.Errorf("flow publish filename %q is invalid", input.Filename)
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Filename)), ".")
	if extension != "tfl" && extension != "tflx" {
		return "", fmt.Errorf("unsupported flow type %q", extension)
	}
	if strings.TrimSpace(input.ContentPath) == "" || input.ContentSize <= 0 || strings.TrimSpace(input.ExpectedFingerprint) == "" {
		return "", errors.New("flow publish requires a native payload path, positive size, and planned fingerprint")
	}
	return extension, nil
}

func publishPayload(name, projectLUID string) ([]byte, error) {
	return xml.Marshal(struct {
		XMLName xml.Name `xml:"tsRequest"`
		Flow    struct {
			Name    string `xml:"name,attr"`
			Project struct {
				ID string `xml:"id,attr"`
			} `xml:"project"`
		} `xml:"flow"`
	}{Flow: struct {
		Name    string `xml:"name,attr"`
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
	}{Name: name, Project: struct {
		ID string `xml:"id,attr"`
	}{ID: projectLUID}}})
}

func moveBody(projectLUID string) ([]byte, error) {
	return xml.Marshal(struct {
		XMLName xml.Name `xml:"tsRequest"`
		Flow    struct {
			Project struct {
				ID string `xml:"id,attr"`
			} `xml:"project"`
		} `xml:"flow"`
	}{Flow: struct {
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
	}{Project: struct {
		ID string `xml:"id,attr"`
	}{ID: projectLUID}}})
}

func uncertainMutation(flowLUID, projectLUID string, err error) (MutationResult, error) {
	if isUncertain(err) {
		return MutationResult{Status: "unknown", FlowLUID: flowLUID, ProjectLUID: projectLUID, TableauRequestID: tableau.RequestID(err)}, err
	}
	return MutationResult{}, err
}

func isUncertain(err error) bool {
	var status interface{ HTTPStatus() int }
	return !errors.As(err, &status) || (status.HTTPStatus() >= http.StatusOK && status.HTTPStatus() < http.StatusMultipleChoices)
}

type paginationXML struct {
	Number *int `xml:"pageNumber,attr"`
	Size   *int `xml:"pageSize,attr"`
	Total  *int `xml:"totalAvailable,attr"`
}

type flowXML struct {
	ID          string `xml:"id,attr"`
	Name        string `xml:"name,attr"`
	Description string `xml:"description,attr"`
	FileType    string `xml:"fileType,attr"`
	CreatedAt   string `xml:"createdAt,attr"`
	UpdatedAt   string `xml:"updatedAt,attr"`
	Project     struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	} `xml:"project"`
	Owner struct {
		ID string `xml:"id,attr"`
	} `xml:"owner"`
	Tags []struct {
		Label string `xml:"label,attr"`
	} `xml:"tags>tag"`
	Parameters []struct {
		ID          string `xml:"id,attr"`
		Name        string `xml:"name,attr"`
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
		Value       string `xml:"value,attr"`
		Required    *bool  `xml:"isRequired,attr"`
	} `xml:"parameters>parameter"`
}

type outputStepXML struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

type listEnvelope struct {
	Pagination paginationXML `xml:"pagination"`
	Flows      []flowXML     `xml:"flows>flow"`
}

type getEnvelope struct {
	Flow        flowXML         `xml:"flow"`
	OutputSteps []outputStepXML `xml:"flowOutputSteps>flowOutputStep"`
}

var _ Client = (*RESTClient)(nil)
var _ MutationClient = (*RESTClient)(nil)
