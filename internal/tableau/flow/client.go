package flow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	defaultPageSize        = 100
	maximumPageSize        = 1000
	metadataResponseLimit  = 4 * 1024 * 1024
	defaultUploadThreshold = 64 * 1024 * 1024
	defaultUploadChunkSize = 64 * 1024 * 1024
	maximumUploadBlocks    = 10_000
)

// RESTClient implements the released Tableau flow REST contract.
type RESTClient struct {
	transport        *tableau.Transport
	session          auth.Session
	serverURL        string
	uploadThreshold  int64
	uploadChunkSize  int64
	maxDownloadBytes int64
}

// NewClient creates an authenticated flow REST client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *RESTClient {
	return &RESTClient{
		transport: transport, session: session, serverURL: serverURL,
		uploadThreshold: defaultUploadThreshold, uploadChunkSize: defaultUploadChunkSize,
	}
}

// SetMaxDownloadBytes bounds the buffered native flow download.
func (c *RESTClient) SetMaxDownloadBytes(limit int64) {
	if c != nil && limit > 0 {
		c.maxDownloadBytes = limit
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
	filters, err := flowFilters(input)
	if err != nil {
		return Page{}, err
	}
	if len(filters) > 0 {
		query.Set("filter", strings.Join(filters, ","))
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

// Download preserves the native TFL or TFLX bytes unchanged.
func (c *RESTClient) Download(ctx context.Context, flowLUID string) (Download, error) {
	if strings.TrimSpace(flowLUID) == "" {
		return Download{}, errors.New("flow LUID is required")
	}
	if err := c.validate(); err != nil {
		return Download{}, err
	}
	response, err := c.do(ctx, http.MethodGet, c.sitePath("flows", flowLUID, "content"), nil, nil, "", "flow.pull", c.maxDownloadBytes)
	if err != nil {
		return Download{}, err
	}
	filename := dispositionFilename(response.Header.Get("Content-Disposition"))
	if filename == "" {
		return Download{}, tableau.NewProtocolError("flow.pull", response, errors.New("flow download response omitted filename"), true)
	}
	if !validFilename(filename) {
		return Download{}, tableau.NewProtocolError("flow.pull", response, fmt.Errorf("flow download returned invalid filename %q", filename), true)
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != ".tfl" && extension != ".tflx" {
		return Download{}, tableau.NewProtocolError("flow.pull", response, fmt.Errorf("flow download returned unsupported filename %q", filename), true)
	}
	return Download{Filename: filename, Content: response.Body, TableauRequestID: response.TableauRequestID}, nil
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
	if err := validateUploadBlockCount(input.ContentSize, c.uploadChunkSize); err != nil {
		return nil, err
	}
	orderedUpload := false
	if input.ContentSize > c.uploadThreshold {
		orderedUpload, err = apiAtLeast(c.transport.APIVersion(), 3, 27)
		if err != nil {
			return nil, fmt.Errorf("interpret Tableau REST API version for flow upload: %w", err)
		}
	}
	source, err := snapshotPublishContent(ctx, input)
	if err != nil {
		return nil, err
	}
	open := true
	closeSource := func() error {
		if !open {
			return nil
		}
		open = false
		return source.close()
	}
	defer func() { _ = closeSource() }()

	var uploadSessionID string
	var content []byte
	if source.size > c.uploadThreshold {
		uploadSessionID, err = c.upload(ctx, input.Filename, source.reader, source.size, orderedUpload)
	} else {
		content, err = readExact(source.reader, source.size)
	}
	if err != nil {
		return nil, errors.Join(err, closeSource())
	}
	inputBody, contentType, err := publishBody(input.Name, input.ProjectLUID, input.Filename, content, uploadSessionID == "")
	if err != nil {
		return nil, errors.Join(err, closeSource())
	}
	if err := closeSource(); err != nil {
		return nil, fmt.Errorf("remove flow publish snapshot: %w", err)
	}
	query := url.Values{"overwrite": {strconv.FormatBool(input.Overwrite)}}
	if uploadSessionID != "" {
		query.Set("uploadSessionId", uploadSessionID)
		query.Set("flowType", extension)
	}
	return &preparedPublish{
		client: c, query: query, body: inputBody, contentType: contentType,
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
	body                []byte
	contentType         string
	expectedName        string
	expectedProjectLUID string
	mu                  sync.Mutex
	committed           bool
}

// Commit sends the final publish request exactly one time.
func (p *preparedPublish) Commit(ctx context.Context) (PublishResult, error) {
	if p == nil || p.client == nil {
		return PublishResult{}, errors.New("flow publish is not prepared")
	}
	p.mu.Lock()
	if p.committed {
		p.mu.Unlock()
		return PublishResult{}, errors.New("prepared flow publish was already committed")
	}
	p.committed = true
	p.mu.Unlock()

	response, err := p.client.do(ctx, http.MethodPost, p.client.sitePath("flows"), p.query, p.body, p.contentType, "flow.publish", metadataResponseLimit)
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

func (c *RESTClient) upload(ctx context.Context, filename string, content io.Reader, size int64, ordered bool) (string, error) {
	response, err := c.do(ctx, http.MethodPost, c.sitePath("fileUploads"), nil, nil, "", "flow.upload.initiate", metadataResponseLimit)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusCreated {
		return "", tableau.NewProtocolError("flow.upload.initiate", response, fmt.Errorf("initiate flow upload returned HTTP %d, expected 201", response.StatusCode), false)
	}
	var initiated uploadEnvelope
	if err := xml.Unmarshal(response.Body, &initiated); err != nil {
		return "", tableau.NewProtocolError("flow.upload.initiate", response, fmt.Errorf("decode initiate flow upload response: %w", err), false)
	}
	if initiated.Upload.SessionID == "" {
		return "", tableau.NewProtocolError("flow.upload.initiate", response, errors.New("initiate flow upload response omitted upload session ID"), false)
	}
	remaining := size
	for sequence := 1; remaining > 0; sequence++ {
		blockSize := min(remaining, c.uploadChunkSize)
		block, err := readBlock(content, blockSize)
		if err != nil {
			return "", err
		}
		remaining -= blockSize
		body, contentType, err := appendBody(filename, block)
		if err != nil {
			return "", err
		}
		query := url.Values{}
		if ordered {
			query.Set("sequenceID", strconv.Itoa(sequence))
		}
		response, err := c.do(ctx, http.MethodPut, c.sitePath("fileUploads", initiated.Upload.SessionID), query, body, contentType, "flow.upload.append", metadataResponseLimit)
		if err != nil {
			return "", err
		}
		if response.StatusCode != http.StatusOK {
			return "", tableau.NewProtocolError("flow.upload.append", response, fmt.Errorf("append flow upload returned HTTP %d, expected 200", response.StatusCode), false)
		}
		var appended uploadEnvelope
		if err := xml.Unmarshal(response.Body, &appended); err != nil {
			return "", tableau.NewProtocolError("flow.upload.append", response, fmt.Errorf("decode append flow upload response: %w", err), false)
		}
		if appended.Upload.SessionID != initiated.Upload.SessionID {
			return "", tableau.NewProtocolError("flow.upload.append", response, fmt.Errorf("append flow upload response returned upload session ID %q, expected %q", appended.Upload.SessionID, initiated.Upload.SessionID), false)
		}
	}
	return initiated.Upload.SessionID, nil
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

func flowFilters(input ListRequest) ([]string, error) {
	fields := []struct{ name, value string }{
		{name: "name", value: input.Name},
		{name: "ownerName", value: input.OwnerName},
		{name: "projectId", value: input.ProjectLUID},
		{name: "projectName", value: input.ProjectName},
	}
	filters := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.value != "" {
			if strings.ContainsAny(field.value, "&,") {
				return nil, fmt.Errorf("flow filter %s cannot contain ampersand or comma", field.name)
			}
			filters = append(filters, field.name+":eq:"+field.value)
		}
	}
	return filters, nil
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
	if !validFilename(input.Filename) {
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

type publishSource struct {
	reader io.ReadSeeker
	size   int64
	close  func() error
}

func snapshotPublishContent(ctx context.Context, input PublishRequest) (publishSource, error) {
	source, err := os.Open(input.ContentPath)
	if err != nil {
		return publishSource{}, fmt.Errorf("open flow publish payload: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return publishSource{}, fmt.Errorf("inspect flow publish payload: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != input.ContentSize {
		return publishSource{}, errors.New("flow publish payload changed after planning")
	}
	snapshot, err := os.CreateTemp(filepath.Dir(input.ContentPath), ".tadx-flow-publish-snapshot-*")
	if err != nil {
		return publishSource{}, fmt.Errorf("create flow publish snapshot: %w", err)
	}
	snapshotName := snapshot.Name()
	cleanup := func() error { return errors.Join(snapshot.Close(), os.Remove(snapshotName)) }
	digest := sha256.New()
	written, err := copyWithContext(ctx, io.MultiWriter(snapshot, digest), source)
	if err != nil {
		return publishSource{}, errors.Join(fmt.Errorf("snapshot flow publish payload: %w", err), cleanup())
	}
	if written != input.ContentSize || "sha256:"+hex.EncodeToString(digest.Sum(nil)) != input.ExpectedFingerprint {
		return publishSource{}, errors.Join(errors.New("flow publish payload changed after planning"), cleanup())
	}
	if _, err := snapshot.Seek(0, io.SeekStart); err != nil {
		return publishSource{}, errors.Join(fmt.Errorf("rewind flow publish snapshot: %w", err), cleanup())
	}
	return publishSource{reader: snapshot, size: written, close: cleanup}, nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

func readExact(reader io.Reader, size int64) ([]byte, error) {
	if size < 0 || size > defaultUploadThreshold {
		return nil, errors.New("invalid flow publish block size")
	}
	content, err := io.ReadAll(io.LimitReader(reader, size+1))
	if err != nil {
		return nil, fmt.Errorf("read flow publish payload: %w", err)
	}
	if int64(len(content)) != size {
		return nil, fmt.Errorf("read %d flow publish bytes, expected %d", len(content), size)
	}
	return content, nil
}

func readBlock(reader io.Reader, size int64) ([]byte, error) {
	if size <= 0 || size > defaultUploadChunkSize {
		return nil, errors.New("invalid flow upload block size")
	}
	content := make([]byte, int(size))
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, fmt.Errorf("read flow upload block: %w", err)
	}
	return content, nil
}

func validateUploadBlockCount(size, chunkSize int64) error {
	if size <= 0 || chunkSize <= 0 {
		return errors.New("invalid flow publish payload size")
	}
	blocks := size / chunkSize
	if size%chunkSize != 0 {
		blocks++
	}
	if blocks > maximumUploadBlocks {
		return fmt.Errorf("flow requires %d upload blocks, exceeding limit %d", blocks, maximumUploadBlocks)
	}
	return nil
}

func publishBody(name, projectLUID, filename string, content []byte, includeFile bool) ([]byte, string, error) {
	requestXML, err := xml.Marshal(struct {
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
	if err != nil {
		return nil, "", err
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	requestPart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`name="request_payload"`}, "Content-Type": {"text/xml"}})
	if err != nil {
		return nil, "", err
	}
	if _, err := requestPart.Write(requestXML); err != nil {
		return nil, "", err
	}
	if includeFile {
		flowPart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {multipartDisposition("tableau_flow", filename)}, "Content-Type": {"application/octet-stream"}})
		if err != nil {
			return nil, "", err
		}
		if _, err := flowPart.Write(content); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "multipart/mixed; boundary=" + writer.Boundary(), nil
}

func appendBody(filename string, content []byte) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	requestPart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`name="request_payload"`}, "Content-Type": {"text/xml"}})
	if err != nil {
		return nil, "", err
	}
	if _, err := requestPart.Write(nil); err != nil {
		return nil, "", err
	}
	filePart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {multipartDisposition("tableau_file", filename)}, "Content-Type": {"application/octet-stream"}})
	if err != nil {
		return nil, "", err
	}
	if _, err := filePart.Write(content); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "multipart/mixed; boundary=" + writer.Boundary(), nil
}

func multipartDisposition(name, filename string) string {
	formatted := mime.FormatMediaType("form-data", map[string]string{"filename": filepath.Base(filename)})
	return fmt.Sprintf(`name="%s"; %s`, name, strings.TrimPrefix(formatted, "form-data; "))
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

func dispositionFilename(value string) string {
	for _, candidate := range []string{value, "attachment; " + value} {
		_, parameters, err := mime.ParseMediaType(candidate)
		if err == nil && parameters["filename"] != "" {
			return parameters["filename"]
		}
	}
	return ""
}

func validFilename(filename string) bool {
	return filename != "." && filename != ".." && !strings.ContainsAny(filename, `/\`) && filepath.Base(filename) == filename
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

func apiAtLeast(value string, major, minor int) (bool, error) {
	parts := strings.SplitN(value, ".", 3)
	if len(parts) < 2 {
		return false, fmt.Errorf("malformed Tableau REST API version %q", value)
	}
	gotMajor, majorErr := strconv.Atoi(parts[0])
	gotMinor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false, fmt.Errorf("malformed Tableau REST API version %q", value)
	}
	return gotMajor > major || (gotMajor == major && gotMinor >= minor), nil
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

type uploadEnvelope struct {
	Upload struct {
		SessionID string `xml:"uploadSessionId,attr"`
	} `xml:"fileUpload"`
}

var _ Client = (*RESTClient)(nil)
var _ MutationClient = (*RESTClient)(nil)
