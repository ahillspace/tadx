// Package workbook implements the released Tableau workbook REST client family.
package workbook

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
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
	maxUploadBlocks        = 1000
)

// Page is the normalized classic REST page.
type Page = tableau.Page

// Workbook is the workbook identity projection needed by lifecycle actions.
type Workbook struct {
	LUID             string
	Name             string
	ContentURL       string
	ProjectLUID      string
	ProjectName      string
	OwnerLUID        string
	Description      string
	CreatedAt        string
	UpdatedAt        string
	Tags             []string
	TableauRequestID string
}

// WorkbookPage contains one normalized upstream page.
type WorkbookPage struct {
	Page             Page
	Items            []Workbook
	TableauRequestID string
}

// ListRequest selects one bounded REST workbook page.
type ListRequest struct {
	PageNumber  int
	PageSize    int
	Name        string
	OwnerName   string
	ProjectLUID string
	ProjectName string
	Tag         string
}

// Project contains enough hierarchy data to build exact project paths.
type Project struct {
	LUID       string
	Name       string
	ParentLUID string
}

// ProjectPage contains one normalized upstream page.
type ProjectPage struct {
	Page  Page
	Items []Project
}

// Download is a preserved native workbook response.
type Download struct {
	Filename         string
	ContentType      string
	TableauRequestID string
}

// PublishRequest contains only explicit publish choices.
type PublishRequest struct {
	Name                string
	ProjectLUID         string
	Filename            string
	ContentPath         string
	ContentSize         int64
	ExpectedFingerprint string
	Overwrite           bool
}

// PublishResult is the authoritative terminal publish outcome.
type PublishResult struct {
	Status           string
	WorkbookLUID     string
	WorkbookName     string
	ProjectLUID      string
	TableauRequestID string
	Warnings         []ValidationIssue
}

// MutationResult is the authoritative outcome of one workbook mutation.
type MutationResult struct {
	Status           string
	WorkbookLUID     string
	WorkbookName     string
	ProjectLUID      string
	OwnerLUID        string
	Description      *string
	EvidenceSource   string
	TableauRequestID string
}

// UpdateRequest contains only explicit workbook metadata changes.
type UpdateRequest struct {
	LUID        string
	Description *string
	Name        *string
	ProjectLUID *string
	OwnerLUID   *string
}

// ValidationIssue is one server-side TWB validation diagnostic.
type ValidationIssue struct {
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	ElementName string `json:"elementName,omitempty"`
}

// ValidationError reports TWB errors that stop publication.
type ValidationError struct {
	Errors           []ValidationIssue
	Warnings         []ValidationIssue
	TableauRequestID string
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "Tableau workbook validation failed"
	}
	return "Tableau workbook validation failed: " + e.Errors[0].Message
}

// RequestID returns the validation request identifier.
func (e *ValidationError) RequestID() string {
	if e == nil {
		return ""
	}
	return e.TableauRequestID
}

// PreparedPublish holds an uploaded, validated workbook until the final publish
// request. It keeps the publish snapshot until Commit or Close releases it.
type PreparedPublish struct {
	client   *Client
	query    url.Values
	body     *tableau.MultipartBody
	snapshot *tableau.PublishSnapshot
	warnings []ValidationIssue
}

// Client is the first released REST client family.
type Client struct {
	transport       *tableau.Transport
	session         auth.Session
	serverURL       string
	uploadThreshold int
	uploadChunkSize int
}

// NewClient creates an authenticated workbook REST client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{
		transport: transport, session: session, serverURL: serverURL,
		uploadThreshold: defaultUploadThreshold, uploadChunkSize: defaultUploadChunkSize,
	}
}

// Update changes only explicit fields on one exact workbook.
func (c *Client) Update(ctx context.Context, input UpdateRequest) (MutationResult, error) {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return MutationResult{}, errors.New("authenticated workbook client is not configured")
	}
	input.LUID = strings.TrimSpace(input.LUID)
	if input.LUID == "" {
		return MutationResult{}, errors.New("workbook update requires an exact workbook LUID")
	}
	if input.Name == nil && input.ProjectLUID == nil && input.OwnerLUID == nil && input.Description == nil {
		return MutationResult{}, errors.New("workbook update requires at least one explicit field")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return MutationResult{}, errors.New("workbook update name cannot be empty")
	}
	if input.ProjectLUID != nil && strings.TrimSpace(*input.ProjectLUID) == "" {
		return MutationResult{}, errors.New("workbook update project LUID cannot be empty")
	}
	if input.OwnerLUID != nil && strings.TrimSpace(*input.OwnerLUID) == "" {
		return MutationResult{}, errors.New("workbook update owner LUID cannot be empty")
	}
	if input.ProjectLUID != nil {
		value := strings.TrimSpace(*input.ProjectLUID)
		input.ProjectLUID = &value
	}
	if input.OwnerLUID != nil {
		value := strings.TrimSpace(*input.OwnerLUID)
		input.OwnerLUID = &value
	}
	body, err := xml.Marshal(workbookUpdateEnvelope{Workbook: workbookUpdateXML{Name: input.Name, Description: input.Description, Project: optionalLUIDXML(input.ProjectLUID), Owner: optionalLUIDXML(input.OwnerLUID)}})
	if err != nil {
		return MutationResult{}, fmt.Errorf("encode workbook update request: %w", err)
	}
	response, err := c.do(ctx, http.MethodPut, c.sitePath("workbooks", input.LUID), nil, body, "application/xml", "workbook.update")
	if err != nil {
		return MutationResult{Status: "unknown", WorkbookLUID: input.LUID, TableauRequestID: tableau.RequestID(err)}, err
	}
	if response.StatusCode != http.StatusOK {
		result := MutationResult{Status: "unknown", WorkbookLUID: input.LUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("workbook.update", response, fmt.Errorf("workbook update returned HTTP %d, expected 200", response.StatusCode), false)
	}
	var envelope workbookGetEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		result := MutationResult{Status: "unknown", WorkbookLUID: input.LUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("workbook.update", response, fmt.Errorf("decode workbook update response: %w", err), false)
	}
	workbook := normalizeWorkbook(envelope.Workbook)
	var confirmedDescription *string
	if input.Description != nil {
		var evidence struct {
			Workbook struct {
				Description *string `xml:"description,attr"`
			} `xml:"workbook"`
		}
		if err := xml.Unmarshal(response.Body, &evidence); err != nil || evidence.Workbook.Description == nil || *evidence.Workbook.Description != *input.Description {
			return MutationResult{Status: "unknown", WorkbookLUID: input.LUID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("workbook.update", response, errors.New("workbook update response did not confirm the requested description"), false)
		}
		confirmedDescription = evidence.Workbook.Description
	}
	if workbook.LUID != input.LUID || workbook.Name == "" || workbook.ProjectLUID == "" || workbook.OwnerLUID == "" {
		return MutationResult{Status: "unknown", WorkbookLUID: input.LUID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("workbook.update", response, errors.New("workbook update response omitted or changed authoritative identity"), false)
	}
	if (input.Name != nil && workbook.Name != *input.Name) || (input.ProjectLUID != nil && workbook.ProjectLUID != *input.ProjectLUID) || (input.OwnerLUID != nil && workbook.OwnerLUID != *input.OwnerLUID) {
		return MutationResult{Status: "unknown", WorkbookLUID: workbook.LUID, WorkbookName: workbook.Name, ProjectLUID: workbook.ProjectLUID, OwnerLUID: workbook.OwnerLUID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("workbook.update", response, errors.New("workbook update response did not preserve the requested changes"), false)
	}
	result := MutationResult{Status: "succeeded", WorkbookLUID: workbook.LUID, WorkbookName: workbook.Name, ProjectLUID: workbook.ProjectLUID, OwnerLUID: workbook.OwnerLUID, TableauRequestID: response.TableauRequestID}
	if confirmedDescription != nil {
		result.Description = confirmedDescription
		result.EvidenceSource = "tableau_update_response"
	}
	return result, nil
}

// Delete removes one exact workbook and accepts only Tableau's documented empty HTTP 204 response.
func (c *Client) Delete(ctx context.Context, workbookLUID string) (MutationResult, error) {
	if strings.TrimSpace(workbookLUID) == "" {
		return MutationResult{}, errors.New("workbook delete requires an exact workbook LUID")
	}
	response, err := c.do(ctx, http.MethodDelete, c.sitePath("workbooks", workbookLUID), nil, nil, "", "workbook.delete")
	if err != nil {
		return MutationResult{Status: "unknown", WorkbookLUID: workbookLUID, TableauRequestID: tableau.RequestID(err)}, err
	}
	if response.StatusCode != http.StatusNoContent || len(response.Body) != 0 {
		result := MutationResult{Status: "unknown", WorkbookLUID: workbookLUID, TableauRequestID: response.TableauRequestID}
		return result, tableau.NewProtocolError("workbook.delete", response, fmt.Errorf("workbook delete returned HTTP %d with %d response bytes, expected empty HTTP 204", response.StatusCode, len(response.Body)), false)
	}
	return MutationResult{Status: "succeeded", WorkbookLUID: workbookLUID, TableauRequestID: response.TableauRequestID}, nil
}

// SetUploadThreshold overrides the single-request threshold for deterministic tests.
func (c *Client) SetUploadThreshold(bytes int) {
	if bytes > 0 && bytes <= defaultUploadThreshold {
		c.uploadThreshold = bytes
	}
}

// SetUploadChunkSize overrides the upload block size for deterministic tests.
func (c *Client) SetUploadChunkSize(bytes int) {
	if bytes > 0 && bytes <= defaultUploadChunkSize {
		c.uploadChunkSize = bytes
	}
}

// List returns one classic REST workbook page.
func (c *Client) List(ctx context.Context, pageNumber, pageSize int) (WorkbookPage, error) {
	return c.ListWorkbooks(ctx, ListRequest{PageNumber: pageNumber, PageSize: pageSize})
}

// ListWorkbooks returns one filtered classic REST workbook page.
func (c *Client) ListWorkbooks(ctx context.Context, input ListRequest) (WorkbookPage, error) {
	if input.PageNumber == 0 {
		input.PageNumber = 1
	}
	if input.PageSize == 0 {
		input.PageSize = defaultPageSize
	}
	if input.PageNumber < 1 {
		return WorkbookPage{}, errors.New("workbook list page number must be positive")
	}
	if input.PageSize < 1 || input.PageSize > maximumPageSize {
		return WorkbookPage{}, fmt.Errorf("workbook list page size must be between 1 and %d", maximumPageSize)
	}
	query := url.Values{"pageNumber": {strconv.Itoa(input.PageNumber)}, "pageSize": {strconv.Itoa(input.PageSize)}, "sort": {"name:asc,updatedAt:asc"}}
	filter, err := ListFilter(input)
	if err != nil {
		return WorkbookPage{}, err
	}
	if filter != "" {
		query.Set("filter", filter)
	}
	response, err := c.doMetadata(ctx, http.MethodGet, c.sitePath("workbooks"), query, "workbook.list")
	if err != nil {
		return WorkbookPage{}, err
	}
	var envelope workbookListEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return WorkbookPage{}, tableau.NewProtocolError("workbook.list", response, fmt.Errorf("decode workbook list response: %w", err), true)
	}
	items := make([]Workbook, len(envelope.Workbooks))
	for index, item := range envelope.Workbooks {
		items[index] = normalizeWorkbook(item)
		items[index].TableauRequestID = response.TableauRequestID
	}
	page, err := normalizePagination(envelope.Pagination, input.PageNumber, input.PageSize, len(items))
	if err != nil {
		return WorkbookPage{}, tableau.NewProtocolError("workbook.list", response, err, true)
	}
	return WorkbookPage{Page: page, Items: items, TableauRequestID: response.TableauRequestID}, nil
}

// Get returns one workbook by its authoritative LUID.
func (c *Client) Get(ctx context.Context, workbookLUID string) (Workbook, error) {
	if workbookLUID == "" {
		return Workbook{}, errors.New("workbook LUID is required")
	}
	response, err := c.doMetadata(ctx, http.MethodGet, c.sitePath("workbooks", workbookLUID), nil, "workbook.get")
	if err != nil {
		return Workbook{}, err
	}
	var envelope workbookGetEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Workbook{}, tableau.NewProtocolError("workbook.get", response, fmt.Errorf("decode workbook response: %w", err), true)
	}
	workbook := normalizeWorkbook(envelope.Workbook)
	workbook.TableauRequestID = response.TableauRequestID
	if workbook.LUID == "" || workbook.LUID != workbookLUID {
		return Workbook{}, tableau.NewProtocolError("workbook.get", response, fmt.Errorf("workbook response returned LUID %q, expected %q", workbook.LUID, workbookLUID), true)
	}
	return workbook, nil
}

// ListProjects returns one classic REST project page.
func (c *Client) ListProjects(ctx context.Context, pageNumber, pageSize int) (ProjectPage, error) {
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	query := url.Values{"pageNumber": {strconv.Itoa(pageNumber)}, "pageSize": {strconv.Itoa(pageSize)}}
	response, err := c.do(ctx, http.MethodGet, c.sitePath("projects"), query, nil, "", "project.list")
	if err != nil {
		return ProjectPage{}, err
	}
	var envelope projectListEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return ProjectPage{}, tableau.NewProtocolError("project.list", response, fmt.Errorf("decode project list response: %w", err), true)
	}
	items := make([]Project, len(envelope.Projects))
	for index, item := range envelope.Projects {
		items[index] = Project{LUID: item.ID, Name: item.Name, ParentLUID: item.ParentProjectID}
	}
	page, err := normalizePagination(envelope.Pagination, pageNumber, pageSize, len(items))
	if err != nil {
		return ProjectPage{}, tableau.NewProtocolError("project.list", response, err, true)
	}
	return ProjectPage{Page: page, Items: items}, nil
}

// Download streams the native TWB or TWBX bytes unchanged into destination
// and returns the filename. On error destination may hold a partial body.
func (c *Client) Download(ctx context.Context, workbookLUID string, includeExtract *bool, destination io.Writer) (Download, error) {
	if c == nil || c.transport == nil || c.session == nil {
		return Download{}, errors.New("authenticated workbook client is not configured")
	}
	query := url.Values{}
	if includeExtract != nil {
		query.Set("includeExtract", strconv.FormatBool(*includeExtract))
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodGet, ServerURL: c.serverURL, Path: c.sitePath("workbooks", workbookLUID, "content"),
		Query: query, Accept: "application/xml", Operation: "workbook.pull", MaxResponseBytes: metadataResponseLimit, ResponseBody: destination,
	})
	if err != nil {
		return Download{}, err
	}
	filename := tableau.DispositionFilename(response.Header.Get("Content-Disposition"))
	if filename == "" {
		return Download{}, tableau.NewProtocolError("workbook.pull", response, errors.New("workbook download response omitted filename"), true)
	}
	if !tableau.ValidFilename(filename) {
		return Download{}, tableau.NewProtocolError("workbook.pull", response, fmt.Errorf("workbook download returned invalid filename %q", filename), true)
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != ".twb" && extension != ".twbx" {
		return Download{}, tableau.NewProtocolError("workbook.pull", response, fmt.Errorf("workbook download returned unsupported filename %q", filename), true)
	}
	return Download{Filename: filename, ContentType: response.Header.Get("Content-Type"), TableauRequestID: response.TableauRequestID}, nil
}

// Publish uploads a workbook and publishes it with one synchronous request.
func (c *Client) Publish(ctx context.Context, input PublishRequest) (PublishResult, error) {
	prepared, err := c.Prepare(ctx, input)
	if err != nil {
		return PublishResult{}, err
	}
	return prepared.Commit(ctx)
}

// Prepare validates and uploads a workbook without issuing the final publish request.
func (c *Client) Prepare(ctx context.Context, input PublishRequest) (*PreparedPublish, error) {
	if input.Name == "" || input.ProjectLUID == "" || input.Filename == "" || input.ContentPath == "" {
		return nil, errors.New("workbook publish requires name, project LUID, filename, and payload path")
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Filename)), ".")
	if extension != "twb" && extension != "twbx" {
		return nil, fmt.Errorf("unsupported workbook type %q", extension)
	}
	if input.ContentSize > int64(c.uploadThreshold) {
		if err := validateUploadBlocks(input.ContentSize, c.uploadChunkSize); err != nil {
			return nil, err
		}
	}
	snapshot, err := tableau.SnapshotPublishPayload(ctx, input.ContentPath, input.ContentSize, input.ExpectedFingerprint)
	if err != nil {
		return nil, fmt.Errorf("prepare workbook publish payload: %w", err)
	}
	prepared := false
	defer func() {
		if !prepared {
			_ = snapshot.Close()
		}
	}()
	var warnings []ValidationIssue
	if extension == "twb" {
		warnings, err = c.validateWorkbook(ctx, input.Filename, snapshot)
		if err != nil {
			return nil, err
		}
	}
	payload, err := publishPayload(input)
	if err != nil {
		return nil, err
	}
	parts := []tableau.MultipartPart{tableau.RequestPayloadPart(payload)}
	query := url.Values{"overwrite": {strconv.FormatBool(input.Overwrite)}}
	if snapshot.Size() > int64(c.uploadThreshold) {
		upload := tableau.FileUpload{Transport: c.transport, Session: c.session, ServerURL: c.serverURL, Operation: "workbook", Filename: input.Filename, ChunkSize: int64(c.uploadChunkSize)}
		uploadSessionID, err := upload.Upload(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		query.Set("uploadSessionId", uploadSessionID)
		query.Set("workbookType", extension)
	} else {
		parts = append(parts, tableau.FilePart("tableau_workbook", input.Filename, snapshot.File(), 0, snapshot.Size()))
	}
	body, err := tableau.NewMultipartBody(parts...)
	if err != nil {
		return nil, err
	}
	prepared = true
	return &PreparedPublish{client: c, query: query, body: body, snapshot: snapshot, warnings: warnings}, nil
}

// Commit issues the final publish request exactly once, then releases the publish
// snapshot. A snapshot cleanup failure never changes the publish result.
func (p *PreparedPublish) Commit(ctx context.Context) (PublishResult, error) {
	if p == nil || p.client == nil {
		return PublishResult{}, errors.New("workbook publish is not prepared")
	}
	if err := p.snapshot.Claim(); err != nil {
		return PublishResult{}, fmt.Errorf("prepared workbook publish: %w", err)
	}
	defer func() { _ = p.snapshot.Close() }()

	response, err := p.client.doMultipart(ctx, http.MethodPost, p.client.sitePath("workbooks"), p.query, p.body, "application/xml", "workbook.publish")
	if err != nil {
		var status interface{ HTTPStatus() int }
		if !errors.As(err, &status) || (status.HTTPStatus() >= http.StatusOK && status.HTTPStatus() < http.StatusMultipleChoices) {
			return PublishResult{Status: "unknown", TableauRequestID: tableau.RequestID(err), Warnings: append([]ValidationIssue(nil), p.warnings...)}, err
		}
		return PublishResult{}, err
	}
	result, err := parsePublishResponse(response)
	result.Warnings = append([]ValidationIssue(nil), p.warnings...)
	result.TableauRequestID = response.TableauRequestID
	if err != nil {
		result.Status = "unknown"
		return result, tableau.NewProtocolError("workbook.publish", response, err, false)
	}
	return result, nil
}

// Close releases the publish snapshot when Commit never runs.
func (p *PreparedPublish) Close() error { return p.snapshot.Close() }

func validateUploadBlocks(size int64, chunkSize int) error {
	if size < 0 || chunkSize <= 0 {
		return errors.New("invalid workbook publish payload size")
	}
	blocks := size / int64(chunkSize)
	if size%int64(chunkSize) != 0 {
		blocks++
	}
	if blocks > maxUploadBlocks {
		return fmt.Errorf("workbook requires %d upload blocks, exceeding conservative limit %d", blocks, maxUploadBlocks)
	}
	return nil
}

func (c *Client) validateWorkbook(ctx context.Context, filename string, snapshot *tableau.PublishSnapshot) ([]ValidationIssue, error) {
	body, err := tableau.NewMultipartBody(tableau.FilePart("tableau_workbook", filename, snapshot.File(), 0, snapshot.Size()))
	if err != nil {
		return nil, fmt.Errorf("build workbook validation request: %w", err)
	}
	response, err := c.doMultipart(ctx, http.MethodPost, c.sitePath("workbooks", "validateWorkbook"), nil, body, "application/json", "workbook.validate")
	if err != nil {
		var upstream *tableau.UpstreamError
		if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusUnprocessableEntity {
			return nil, err
		}
		var envelope validationEnvelope
		if json.Unmarshal([]byte(upstream.Detail), &envelope) != nil || len(envelope.Errors) == 0 {
			return nil, err
		}
		return envelope.Warnings, &ValidationError{Errors: envelope.Errors, Warnings: envelope.Warnings, TableauRequestID: upstream.TableauRequestID}
	}
	var envelope validationEnvelope
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return nil, tableau.NewProtocolError("workbook.validate", response, fmt.Errorf("decode workbook validation response: %w", err), false)
	}
	if len(envelope.Errors) > 0 {
		return envelope.Warnings, &ValidationError{Errors: envelope.Errors, Warnings: envelope.Warnings, TableauRequestID: response.TableauRequestID}
	}
	return envelope.Warnings, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body []byte, contentType, operation string) (tableau.Response, error) {
	if c == nil || c.transport == nil || c.session == nil {
		return tableau.Response{}, errors.New("authenticated workbook client is not configured")
	}
	return c.transport.Do(ctx, c.session, tableau.Request{
		Method: method, ServerURL: c.serverURL, Path: path, Query: query, Body: body,
		ContentType: contentType, Accept: "application/xml", Operation: operation,
	})
}

func (c *Client) doMetadata(ctx context.Context, method, path string, query url.Values, operation string) (tableau.Response, error) {
	if c == nil || c.transport == nil || c.session == nil {
		return tableau.Response{}, errors.New("authenticated workbook client is not configured")
	}
	return c.transport.Do(ctx, c.session, tableau.Request{Method: method, ServerURL: c.serverURL, Path: path, Query: query, Accept: "application/xml", Operation: operation, MaxResponseBytes: metadataResponseLimit})
}

func (c *Client) doMultipart(ctx context.Context, method, path string, query url.Values, body *tableau.MultipartBody, accept, operation string) (tableau.Response, error) {
	if c == nil || c.transport == nil || c.session == nil {
		return tableau.Response{}, errors.New("authenticated workbook client is not configured")
	}
	return c.transport.Do(ctx, c.session, tableau.Request{
		Method: method, ServerURL: c.serverURL, Path: path, Query: query, Multipart: body,
		Accept: accept, Operation: operation,
	})
}

func (c *Client) sitePath(parts ...string) string {
	segments := []string{"api", c.transport.APIVersion(), "sites", c.session.SiteLUID()}
	segments = append(segments, parts...)
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return "/" + strings.Join(segments, "/")
}

func publishPayload(input PublishRequest) ([]byte, error) {
	return xml.Marshal(struct {
		XMLName  xml.Name `xml:"tsRequest"`
		Workbook struct {
			Name    string `xml:"name,attr"`
			Project struct {
				ID string `xml:"id,attr"`
			} `xml:"project"`
		} `xml:"workbook"`
	}{Workbook: struct {
		Name    string `xml:"name,attr"`
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
	}{Name: input.Name, Project: struct {
		ID string `xml:"id,attr"`
	}{ID: input.ProjectLUID}}})
}

// parsePublishResponse accepts only a completed synchronous publish. tadx
// never requests a job, so a job response leaves the outcome unknown.
func parsePublishResponse(response tableau.Response) (PublishResult, error) {
	var envelope publishEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return PublishResult{
			WorkbookLUID: envelope.Workbook.ID, WorkbookName: envelope.Workbook.Name, ProjectLUID: envelope.Workbook.Project.ID,
		}, fmt.Errorf("decode workbook publish response: %w", err)
	}
	if envelope.Job.ID != "" || response.StatusCode == http.StatusAccepted {
		return PublishResult{}, fmt.Errorf("Tableau returned asynchronous job %q for a synchronous workbook publish", envelope.Job.ID)
	}
	if envelope.Workbook.ID == "" {
		return PublishResult{}, errors.New("workbook publish response omitted workbook identity")
	}
	return PublishResult{Status: "succeeded", WorkbookLUID: envelope.Workbook.ID, WorkbookName: envelope.Workbook.Name, ProjectLUID: envelope.Workbook.Project.ID}, nil
}

type paginationXML struct {
	Number *int `xml:"pageNumber,attr"`
	Size   *int `xml:"pageSize,attr"`
	Total  *int `xml:"totalAvailable,attr"`
}

type workbookListEnvelope struct {
	Pagination *paginationXML `xml:"pagination"`
	Workbooks  []workbookXML  `xml:"workbooks>workbook"`
}

type workbookGetEnvelope struct {
	Workbook workbookXML `xml:"workbook"`
}

type workbookUpdateEnvelope struct {
	XMLName  xml.Name          `xml:"tsRequest"`
	Workbook workbookUpdateXML `xml:"workbook"`
}

type workbookUpdateXML struct {
	Description *string  `xml:"description,attr,omitempty"`
	Name        *string  `xml:"name,attr,omitempty"`
	Project     *luidXML `xml:"project,omitempty"`
	Owner       *luidXML `xml:"owner,omitempty"`
}

type luidXML struct {
	ID string `xml:"id,attr"`
}

func optionalLUIDXML(value *string) *luidXML {
	if value == nil {
		return nil
	}
	return &luidXML{ID: strings.TrimSpace(*value)}
}

type workbookXML struct {
	ID          string `xml:"id,attr"`
	Name        string `xml:"name,attr"`
	ContentURL  string `xml:"contentUrl,attr"`
	Description string `xml:"description,attr"`
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
}

type projectListEnvelope struct {
	Pagination *paginationXML `xml:"pagination"`
	Projects   []struct {
		ID              string `xml:"id,attr"`
		Name            string `xml:"name,attr"`
		ParentProjectID string `xml:"parentProjectId,attr"`
	} `xml:"projects>project"`
}

func normalizeWorkbook(item workbookXML) Workbook {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		label := strings.TrimSpace(tag.Label)
		if label != "" {
			tags = append(tags, label)
		}
	}
	sort.Strings(tags)
	return Workbook{LUID: item.ID, Name: item.Name, ContentURL: item.ContentURL, ProjectLUID: item.Project.ID, ProjectName: item.Project.Name, OwnerLUID: item.Owner.ID, Description: item.Description, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Tags: tags}
}

// ListFilter validates and encodes the workbook selectors supported by the
// Tableau workbooks REST endpoint for paged and full lists.
//
// Tableau does not support projectId on this endpoint. Callers that receive a
// project LUID must apply that exact identity check to the returned records
// while traversing the complete upstream result set.
// Pagination fields do not affect the selected population.
func ListFilter(input ListRequest) (string, error) {
	if strings.ContainsAny(input.ProjectLUID, "&,") {
		return "", errors.New("workbook project ID filter cannot contain ampersand or comma")
	}
	fields := []struct{ name, value string }{
		{name: "name", value: input.Name},
		{name: "ownerName", value: input.OwnerName},
		{name: "projectName", value: input.ProjectName},
		{name: "tags", value: input.Tag},
	}
	filters := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		if strings.ContainsAny(field.value, "&,") {
			return "", fmt.Errorf("workbook filter %s cannot contain ampersand or comma", field.name)
		}
		filters = append(filters, field.name+":eq:"+field.value)
	}
	return strings.Join(filters, ","), nil
}

func normalizePagination(value *paginationXML, requestedNumber, requestedSize, itemCount int) (Page, error) {
	if value == nil || value.Number == nil || value.Size == nil || value.Total == nil {
		return Page{}, errors.New("Tableau list response omitted pagination")
	}
	number, size, total := *value.Number, *value.Size, *value.Total
	if number != requestedNumber || number <= 0 {
		return Page{}, fmt.Errorf("Tableau list response returned page number %d, expected %d", number, requestedNumber)
	}
	if size <= 0 || size > requestedSize {
		return Page{}, fmt.Errorf("Tableau list response returned invalid page size %d", size)
	}
	if total < 0 || itemCount > size || total < itemCount {
		return Page{}, fmt.Errorf("Tableau list response returned inconsistent pagination total %d, size %d, and item count %d", total, size, itemCount)
	}
	pageIndex := int64(number - 1)
	if pageIndex > int64(total)/int64(size) {
		return Page{}, fmt.Errorf("Tableau list response total %d is inconsistent with page %d", total, number)
	}
	offset := pageIndex * int64(size)
	remaining := int64(total) - offset
	if remaining < 0 {
		return Page{}, fmt.Errorf("Tableau list response total %d is inconsistent with page %d", total, number)
	}
	expected := min(int64(size), remaining)
	if int64(itemCount) != expected {
		return Page{}, fmt.Errorf("Tableau list response returned %d items for page %d; expected %d from total %d and size %d", itemCount, number, expected, total, size)
	}
	return Page{Number: number, Size: size, Total: total}, nil
}

type publishEnvelope struct {
	Workbook struct {
		ID      string `xml:"id,attr"`
		Name    string `xml:"name,attr"`
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
	} `xml:"workbook"`
	Job struct {
		ID string `xml:"id,attr"`
	} `xml:"job"`
}

type validationEnvelope struct {
	Errors   []ValidationIssue `json:"errors"`
	Warnings []ValidationIssue `json:"warnings"`
}
