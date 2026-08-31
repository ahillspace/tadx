// Package workbook implements the released Tableau workbook REST client family.
package workbook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"time"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	defaultPageSize        = 100
	defaultUploadThreshold = 64 * 1024 * 1024
	defaultUploadChunkSize = 64 * 1024 * 1024
	maxUploadBlocks        = 1000
)

// Page is the normalized classic REST page.
type Page = tableau.Page

// Workbook is the workbook identity projection needed by lifecycle actions.
type Workbook struct {
	LUID        string
	Name        string
	ContentURL  string
	ProjectLUID string
	ProjectName string
	OwnerLUID   string
}

// WorkbookPage contains one normalized upstream page.
type WorkbookPage struct {
	Page  Page
	Items []Workbook
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
	Content          []byte
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
	Content             []byte
	Overwrite           bool
	AsJob               bool
}

// PublishResult is the authoritative terminal publish outcome.
type PublishResult struct {
	Status           string
	WorkbookLUID     string
	WorkbookName     string
	ProjectLUID      string
	JobID            string
	TableauRequestID string
	Warnings         []ValidationIssue
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

// PreparedPublish holds an uploaded, validated workbook until the final publish request.
type PreparedPublish struct {
	client      *Client
	query       url.Values
	body        []byte
	contentType string
	mu          sync.Mutex
	committed   bool
	warnings    []ValidationIssue
}

// Client is the first released REST client family.
type Client struct {
	transport        *tableau.Transport
	session          auth.Session
	serverURL        string
	uploadThreshold  int
	uploadChunkSize  int
	pollInterval     time.Duration
	pollTimeout      time.Duration
	maxDownloadBytes int64
}

// NewClient creates an authenticated workbook REST client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{
		transport: transport, session: session, serverURL: serverURL,
		uploadThreshold: defaultUploadThreshold, uploadChunkSize: defaultUploadChunkSize,
		pollInterval: time.Second, pollTimeout: 10 * time.Minute,
	}
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

// SetMaxDownloadBytes bounds the buffered workbook download below the shared
// transport ceiling. Zero keeps the default 256 MiB limit.
//
// The download is fully buffered because Download.Content is a []byte consumed
// by internal/artifact and internal/app; a true streaming download (a bounded
// temp file exposed as an io.ReadCloser, validated from disk, with a streaming
// request body in the transport) would ripple those contracts and is deferred.
func (c *Client) SetMaxDownloadBytes(limit int64) {
	if limit > 0 {
		c.maxDownloadBytes = limit
	}
}

// SetPollPolicy configures bounded internal asynchronous polling.
func (c *Client) SetPollPolicy(interval, timeout time.Duration) {
	if interval > 0 {
		c.pollInterval = interval
	}
	if timeout > 0 {
		c.pollTimeout = timeout
	}
}

// List returns one classic REST workbook page.
func (c *Client) List(ctx context.Context, pageNumber, pageSize int) (WorkbookPage, error) {
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	query := url.Values{"pageNumber": {strconv.Itoa(pageNumber)}, "pageSize": {strconv.Itoa(pageSize)}}
	response, err := c.do(ctx, http.MethodGet, c.sitePath("workbooks"), query, nil, "", "workbook.list")
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
	}
	page, err := normalizePagination(envelope.Pagination, pageNumber, pageSize, len(items))
	if err != nil {
		return WorkbookPage{}, tableau.NewProtocolError("workbook.list", response, err, true)
	}
	return WorkbookPage{Page: page, Items: items}, nil
}

// Get returns one workbook by its authoritative LUID.
func (c *Client) Get(ctx context.Context, workbookLUID string) (Workbook, error) {
	if workbookLUID == "" {
		return Workbook{}, errors.New("workbook LUID is required")
	}
	response, err := c.do(ctx, http.MethodGet, c.sitePath("workbooks", workbookLUID), nil, nil, "", "workbook.get")
	if err != nil {
		return Workbook{}, err
	}
	var envelope workbookGetEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Workbook{}, tableau.NewProtocolError("workbook.get", response, fmt.Errorf("decode workbook response: %w", err), true)
	}
	workbook := normalizeWorkbook(envelope.Workbook)
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

// Download preserves the native TWB or TWBX bytes and filename.
//
// The response is buffered into Download.Content ([]byte) up to the configured
// cap (see SetMaxDownloadBytes). Streaming to a bounded temp file would require
// changing Download.Content's type, which ripples into internal/artifact and
// internal/app, so it is intentionally out of scope here.
func (c *Client) Download(ctx context.Context, workbookLUID string, includeExtract *bool) (Download, error) {
	if c == nil || c.transport == nil || c.session == nil {
		return Download{}, errors.New("authenticated workbook client is not configured")
	}
	query := url.Values{}
	if includeExtract != nil {
		query.Set("includeExtract", strconv.FormatBool(*includeExtract))
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodGet, ServerURL: c.serverURL, Path: c.sitePath("workbooks", workbookLUID, "content"),
		Query: query, Accept: "application/xml", Operation: "workbook.pull", MaxResponseBytes: c.maxDownloadBytes,
	})
	if err != nil {
		return Download{}, err
	}
	filename := dispositionFilename(response.Header.Get("Content-Disposition"))
	if filename == "" {
		return Download{}, tableau.NewProtocolError("workbook.pull", response, errors.New("workbook download response omitted filename"), true)
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != ".twb" && extension != ".twbx" {
		return Download{}, tableau.NewProtocolError("workbook.pull", response, fmt.Errorf("workbook download returned unsupported filename %q", filename), true)
	}
	return Download{Filename: filepath.Base(filename), ContentType: response.Header.Get("Content-Type"), Content: response.Body, TableauRequestID: response.TableauRequestID}, nil
}

// Publish uploads and publishes a workbook, then polls an explicit async job to a bounded terminal result.
func (c *Client) Publish(ctx context.Context, input PublishRequest) (PublishResult, error) {
	prepared, err := c.Prepare(ctx, input)
	if err != nil {
		return PublishResult{}, err
	}
	return prepared.Commit(ctx)
}

// Prepare validates and uploads a workbook without issuing the final publish request.
func (c *Client) Prepare(ctx context.Context, input PublishRequest) (*PreparedPublish, error) {
	if input.Name == "" || input.ProjectLUID == "" || input.Filename == "" {
		return nil, errors.New("workbook publish requires name, project LUID, and filename")
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Filename)), ".")
	if extension != "twb" && extension != "twbx" {
		return nil, fmt.Errorf("unsupported workbook type %q", extension)
	}
	plannedSize := int64(len(input.Content))
	if input.ContentPath != "" {
		plannedSize = input.ContentSize
	}
	if plannedSize > int64(c.uploadThreshold) {
		if err := validateUploadBlocks(plannedSize, c.uploadChunkSize); err != nil {
			return nil, err
		}
	}
	source, err := openPublishContent(ctx, input)
	if err != nil {
		return nil, err
	}
	sourceOpen := true
	closeSource := func() error {
		if !sourceOpen {
			return nil
		}
		sourceOpen = false
		return source.close()
	}
	defer func() {
		_ = closeSource()
	}()
	var warnings []ValidationIssue
	if extension == "twb" {
		warnings, err = c.validateWorkbook(ctx, input.Filename, source.reader, source.size)
		if err != nil {
			return nil, err
		}
	}
	var uploadSessionID string
	if source.size > int64(c.uploadThreshold) {
		uploadSessionID, err = c.upload(ctx, input.Filename, source.reader, source.size)
		if err != nil {
			return nil, errors.Join(err, closeSource())
		}
	} else {
		input.Content, err = readPublishContent(source.reader, source.size)
		if err != nil {
			return nil, errors.Join(err, closeSource())
		}
	}
	body, contentType, err := publishBody(input, uploadSessionID == "")
	if err != nil {
		return nil, errors.Join(err, closeSource())
	}
	if err := closeSource(); err != nil {
		return nil, fmt.Errorf("remove workbook publish snapshot: %w", err)
	}
	query := url.Values{"overwrite": {strconv.FormatBool(input.Overwrite)}}
	if input.AsJob {
		query.Set("asJob", "true")
	}
	if uploadSessionID != "" {
		query.Set("uploadSessionId", uploadSessionID)
		query.Set("workbookType", extension)
	}
	return &PreparedPublish{client: c, query: query, body: body, contentType: contentType, warnings: warnings}, nil
}

// Commit issues the final publish request exactly once.
func (p *PreparedPublish) Commit(ctx context.Context) (PublishResult, error) {
	if p == nil || p.client == nil {
		return PublishResult{}, errors.New("workbook publish is not prepared")
	}
	p.mu.Lock()
	if p.committed {
		p.mu.Unlock()
		return PublishResult{}, errors.New("prepared workbook publish was already committed")
	}
	p.committed = true
	p.mu.Unlock()

	response, err := p.client.do(ctx, http.MethodPost, p.client.sitePath("workbooks"), p.query, p.body, p.contentType, "workbook.publish")
	if err != nil {
		var status interface{ HTTPStatus() int }
		if !errors.As(err, &status) || (status.HTTPStatus() >= http.StatusOK && status.HTTPStatus() < http.StatusMultipleChoices) {
			return PublishResult{Status: "unknown", TableauRequestID: tableau.RequestID(err), Warnings: append([]ValidationIssue(nil), p.warnings...)}, err
		}
		return PublishResult{}, err
	}
	result, err := parsePublishResponse(response.Body)
	result.Warnings = append([]ValidationIssue(nil), p.warnings...)
	if err != nil {
		result.Status = "unknown"
		result.TableauRequestID = response.TableauRequestID
		return result, tableau.NewProtocolError("workbook.publish", response, err, false)
	}
	result.TableauRequestID = response.TableauRequestID
	if result.JobID == "" {
		result.Status = "succeeded"
		return result, nil
	}
	terminal, err := p.client.pollJob(ctx, result.JobID)
	terminal.Warnings = append([]ValidationIssue(nil), p.warnings...)
	if terminal.JobID == "" {
		terminal.JobID = result.JobID
	}
	if terminal.TableauRequestID == "" {
		terminal.TableauRequestID = response.TableauRequestID
	}
	if err != nil {
		return terminal, err
	}
	return terminal, nil
}

func (c *Client) upload(ctx context.Context, filename string, content io.Reader, size int64) (string, error) {
	if err := validateUploadBlocks(size, c.uploadChunkSize); err != nil {
		return "", err
	}
	response, err := c.do(ctx, http.MethodPost, c.sitePath("fileUploads"), nil, nil, "", "workbook.upload.initiate")
	if err != nil {
		return "", err
	}
	var initiated fileUploadEnvelope
	if err := xml.Unmarshal(response.Body, &initiated); err != nil {
		return "", tableau.NewProtocolError("workbook.upload.initiate", response, fmt.Errorf("decode initiate file upload response: %w", err), false)
	}
	if initiated.FileUpload.SessionID == "" {
		return "", tableau.NewProtocolError("workbook.upload.initiate", response, errors.New("initiate file upload response omitted upload session ID"), false)
	}
	remaining := size
	for index := 0; remaining > 0; index++ {
		chunkSize := min(remaining, int64(c.uploadChunkSize))
		chunk, err := readPublishContent(content, chunkSize)
		if err != nil {
			return "", err
		}
		remaining -= chunkSize
		body, contentType, err := appendBody(filename, chunk)
		if err != nil {
			return "", err
		}
		query := url.Values{}
		atLeast, err := apiAtLeast(c.transport.APIVersion(), 3, 27)
		if err != nil {
			return "", fmt.Errorf("interpret Tableau REST API version for chunked upload: %w", err)
		}
		if atLeast {
			query.Set("sequenceID", strconv.Itoa(index+1))
		}
		response, err := c.do(ctx, http.MethodPut, c.sitePath("fileUploads", initiated.FileUpload.SessionID), query, body, contentType, "workbook.upload.append")
		if err != nil {
			return "", err
		}
		var appended fileUploadEnvelope
		if err := xml.Unmarshal(response.Body, &appended); err != nil {
			return "", tableau.NewProtocolError("workbook.upload.append", response, fmt.Errorf("decode append upload response: %w", err), false)
		}
		if appended.FileUpload.SessionID != initiated.FileUpload.SessionID {
			return "", tableau.NewProtocolError("workbook.upload.append", response, fmt.Errorf("append upload response returned upload session ID %q, expected %q", appended.FileUpload.SessionID, initiated.FileUpload.SessionID), false)
		}
	}
	return initiated.FileUpload.SessionID, nil
}

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

type publishContent struct {
	reader io.ReadSeeker
	size   int64
	close  func() error
}

func openPublishContent(ctx context.Context, input PublishRequest) (publishContent, error) {
	if input.ContentPath == "" {
		reader := bytes.NewReader(input.Content)
		return publishContent{reader: reader, size: int64(reader.Len()), close: func() error { return nil }}, nil
	}
	if input.ExpectedFingerprint == "" {
		return publishContent{}, errors.New("workbook publish payload requires the planned fingerprint")
	}
	source, err := os.Open(input.ContentPath)
	if err != nil {
		return publishContent{}, fmt.Errorf("open workbook publish payload: %w", err)
	}
	info, err := source.Stat()
	if err != nil {
		_ = source.Close()
		return publishContent{}, fmt.Errorf("inspect workbook publish payload: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = source.Close()
		return publishContent{}, errors.New("workbook publish payload must be a regular file")
	}
	if info.Size() != input.ContentSize {
		_ = source.Close()
		return publishContent{}, fmt.Errorf("workbook publish payload size changed after planning: got %d, expected %d", info.Size(), input.ContentSize)
	}
	snapshot, err := os.CreateTemp(filepath.Dir(input.ContentPath), ".tadx-publish-snapshot-*")
	if err != nil {
		_ = source.Close()
		return publishContent{}, fmt.Errorf("create workbook publish snapshot: %w", err)
	}
	cleanup := func() error {
		return errors.Join(snapshot.Close(), os.Remove(snapshot.Name()))
	}
	fingerprint, copied, err := fingerprintCopy(ctx, snapshot, source)
	sourceCloseErr := source.Close()
	if err != nil {
		return publishContent{}, fmt.Errorf("snapshot workbook publish payload: %w", errors.Join(err, sourceCloseErr, cleanup()))
	}
	if sourceCloseErr != nil {
		return publishContent{}, fmt.Errorf("close workbook publish payload: %w", errors.Join(sourceCloseErr, cleanup()))
	}
	if copied != input.ContentSize {
		sizeErr := fmt.Errorf("workbook publish payload size changed after planning: got %d, expected %d", copied, input.ContentSize)
		return publishContent{}, errors.Join(sizeErr, cleanup())
	}
	if fingerprint != input.ExpectedFingerprint {
		return publishContent{}, errors.Join(errors.New("workbook publish payload changed after planning"), cleanup())
	}
	if _, err := snapshot.Seek(0, io.SeekStart); err != nil {
		return publishContent{}, fmt.Errorf("rewind workbook publish snapshot: %w", errors.Join(err, cleanup()))
	}
	return publishContent{reader: snapshot, size: copied, close: cleanup}, nil
}

func fingerprintCopy(ctx context.Context, destination io.Writer, source io.Reader) (string, int64, error) {
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", total, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			if writeErr != nil {
				return "", total, writeErr
			}
			if written != count {
				return "", total, io.ErrShortWrite
			}
			_, _ = hash.Write(buffer[:count])
			total += int64(count)
		}
		if errors.Is(readErr, io.EOF) {
			return "sha256:" + hex.EncodeToString(hash.Sum(nil)), total, nil
		}
		if readErr != nil {
			return "", total, readErr
		}
	}
}

func readPublishContent(reader io.Reader, size int64) ([]byte, error) {
	if size < 0 || size > int64(^uint(0)>>1) {
		return nil, errors.New("invalid workbook publish payload size")
	}
	content := make([]byte, int(size))
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, fmt.Errorf("read workbook publish payload: %w", err)
	}
	return content, nil
}

func (c *Client) validateWorkbook(ctx context.Context, filename string, reader io.ReadSeeker, size int64) ([]ValidationIssue, error) {
	content, err := readPublishContent(reader, size)
	if err != nil {
		return nil, fmt.Errorf("read workbook for validation: %w", err)
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind workbook after validation: %w", err)
	}
	body, contentType, err := validationBody(filename, content)
	if err != nil {
		return nil, fmt.Errorf("build workbook validation request: %w", err)
	}
	response, err := c.doAccept(ctx, http.MethodPost, c.sitePath("workbooks", "validateWorkbook"), nil, body, contentType, "application/json", "workbook.validate")
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

func (c *Client) pollJob(ctx context.Context, jobID string) (PublishResult, error) {
	pollCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()
	lastRequestID := ""
	for {
		response, err := c.do(pollCtx, http.MethodGet, c.sitePath("jobs", jobID), nil, nil, "", "workbook.publish.poll")
		if err != nil {
			requestID := tableau.RequestID(err)
			if requestID == "" {
				requestID = lastRequestID
			}
			if ctx.Err() != nil {
				return PublishResult{Status: "cancelled", JobID: jobID, TableauRequestID: requestID}, ctx.Err()
			}
			if errors.Is(pollCtx.Err(), context.DeadlineExceeded) {
				return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau workbook publish job %s timed out after %s: %w", jobID, c.pollTimeout, err)
			}
			// A transient upstream failure (429/408/5xx) is not terminal: back off
			// and keep polling until the job resolves or the deadline passes.
			var retryable interface{ Retryable() bool }
			if errors.As(err, &retryable) && retryable.Retryable() {
				delay := c.pollInterval
				if wait, ok := tableau.RetryAfter(err); ok {
					delay = wait
				}
				select {
				case <-pollCtx.Done():
					if ctx.Err() != nil {
						return PublishResult{Status: "cancelled", JobID: jobID, TableauRequestID: requestID}, ctx.Err()
					}
					return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau workbook publish job %s timed out after %s: %w", jobID, c.pollTimeout, err)
				case <-time.After(delay):
				}
				continue
			}
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, err
		}
		if response.TableauRequestID != "" {
			lastRequestID = response.TableauRequestID
		}
		requestID := response.TableauRequestID
		if requestID == "" {
			requestID = lastRequestID
		}
		response.TableauRequestID = requestID
		var envelope jobEnvelope
		if err := xml.Unmarshal(response.Body, &envelope); err != nil {
			// A response can yield its headers just before the polling deadline and
			// then finish with an empty or truncated body as cancellation closes the
			// stream. Preserve the bounded polling outcome instead of misclassifying
			// that timing window as a retryable protocol response.
			if ctx.Err() != nil {
				return PublishResult{Status: "cancelled", JobID: jobID, TableauRequestID: requestID}, ctx.Err()
			}
			if errors.Is(pollCtx.Err(), context.DeadlineExceeded) {
				return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau workbook publish job %s timed out after %s: %w", jobID, c.pollTimeout, err)
			}
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("workbook.publish.poll", response, fmt.Errorf("decode Tableau job response: %w", err), true)
		}
		if envelope.Job.ID != jobID {
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("workbook.publish.poll", response, fmt.Errorf("Tableau job response returned job ID %q, expected %q", envelope.Job.ID, jobID), true)
		}
		if envelope.Job.Type != "PublishWorkbook" {
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("workbook.publish.poll", response, fmt.Errorf("Tableau job response returned type %q, expected %q", envelope.Job.Type, "PublishWorkbook"), true)
		}
		if envelope.Job.Progress == nil {
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("workbook.publish.poll", response, errors.New("Tableau job response omitted progress"), true)
		}
		if envelope.Job.FinishCode == nil {
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("workbook.publish.poll", response, errors.New("Tableau job response omitted finish code"), true)
		}
		progress, finishCode := *envelope.Job.Progress, *envelope.Job.FinishCode
		switch {
		case progress == 0 && finishCode == 1:
			// Tableau documents publish jobs as pending in this exact state.
		case progress == 100 && finishCode == 0:
			return PublishResult{Status: "succeeded", JobID: jobID, TableauRequestID: requestID}, nil
		case progress == 100 && (finishCode == 1 || finishCode == 2):
			return PublishResult{Status: "failed", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau workbook publish job %s failed with finish code %d", jobID, finishCode)
		case progress == 100:
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("workbook.publish.poll", response, fmt.Errorf("Tableau job response returned unknown finish code %d", finishCode), true)
		default:
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("workbook.publish.poll", response, fmt.Errorf("Tableau job response returned invalid state progress=%d finishCode=%d", progress, finishCode), true)
		}
		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return PublishResult{Status: "cancelled", JobID: jobID, TableauRequestID: requestID}, ctx.Err()
			}
			return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau workbook publish job %s timed out after %s", jobID, c.pollTimeout)
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body []byte, contentType, operation string) (tableau.Response, error) {
	return c.doAccept(ctx, method, path, query, body, contentType, "application/xml", operation)
}

func (c *Client) doAccept(ctx context.Context, method, path string, query url.Values, body []byte, contentType, accept, operation string) (tableau.Response, error) {
	if c == nil || c.transport == nil || c.session == nil {
		return tableau.Response{}, errors.New("authenticated workbook client is not configured")
	}
	return c.transport.Do(ctx, c.session, tableau.Request{
		Method: method, ServerURL: c.serverURL, Path: path, Query: query, Body: body,
		ContentType: contentType, Accept: accept, Operation: operation,
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

func dispositionFilename(value string) string {
	_, parameters, err := mime.ParseMediaType("attachment; " + value)
	if err == nil {
		return parameters["filename"]
	}
	for _, part := range strings.Split(value, ";") {
		name, filename, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(name, "filename") {
			return strings.Trim(filename, `"`)
		}
	}
	return ""
}

func publishBody(input PublishRequest, includeFile bool) ([]byte, string, error) {
	requestXML, err := xml.Marshal(struct {
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
	if err != nil {
		return nil, "", err
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	payloadHeader := textproto.MIMEHeader{"Content-Disposition": {`name="request_payload"`}, "Content-Type": {"text/xml"}}
	payload, err := writer.CreatePart(payloadHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := payload.Write(requestXML); err != nil {
		return nil, "", err
	}
	if includeFile {
		fileHeader := textproto.MIMEHeader{"Content-Disposition": {multipartFileDisposition("tableau_workbook", input.Filename)}, "Content-Type": {"application/octet-stream"}}
		part, err := writer.CreatePart(fileHeader)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(input.Content); err != nil {
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
	payload, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`name="request_payload"`}, "Content-Type": {"text/xml"}})
	if err != nil {
		return nil, "", err
	}
	if _, err := io.WriteString(payload, ""); err != nil {
		return nil, "", err
	}
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {multipartFileDisposition("tableau_file", filename)}, "Content-Type": {"application/octet-stream"}})
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(content); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "multipart/mixed; boundary=" + writer.Boundary(), nil
}

func multipartFileDisposition(name, filename string) string {
	formatted := mime.FormatMediaType("form-data", map[string]string{"filename": filepath.Base(filename)})
	return fmt.Sprintf(`name="%s"; %s`, name, strings.TrimPrefix(formatted, "form-data; "))
}

func validationBody(filename string, content []byte) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {multipartFileDisposition("tableau_workbook", filename)},
		"Content-Type":        {"application/octet-stream"},
	})
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(content); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "multipart/mixed; boundary=" + writer.Boundary(), nil
}

func parsePublishResponse(body []byte) (PublishResult, error) {
	var envelope publishEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return PublishResult{
			JobID: envelope.Job.ID, WorkbookLUID: envelope.Workbook.ID,
			WorkbookName: envelope.Workbook.Name, ProjectLUID: envelope.Workbook.Project.ID,
		}, fmt.Errorf("decode workbook publish response: %w", err)
	}
	if envelope.Job.ID != "" {
		return PublishResult{Status: "pending", JobID: envelope.Job.ID}, nil
	}
	if envelope.Workbook.ID == "" {
		return PublishResult{}, errors.New("workbook publish response omitted workbook and job identity")
	}
	return PublishResult{Status: "succeeded", WorkbookLUID: envelope.Workbook.ID, WorkbookName: envelope.Workbook.Name, ProjectLUID: envelope.Workbook.Project.ID}, nil
}

func apiAtLeast(value string, major, minor int) (bool, error) {
	parts := strings.SplitN(value, ".", 3)
	if len(parts) < 2 {
		return false, fmt.Errorf("malformed Tableau REST API version %q", value)
	}
	gotMajor, errMajor := strconv.Atoi(parts[0])
	gotMinor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return false, fmt.Errorf("malformed Tableau REST API version %q", value)
	}
	return gotMajor > major || (gotMajor == major && gotMinor >= minor), nil
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

type workbookXML struct {
	ID         string `xml:"id,attr"`
	Name       string `xml:"name,attr"`
	ContentURL string `xml:"contentUrl,attr"`
	Project    struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	} `xml:"project"`
	Owner struct {
		ID string `xml:"id,attr"`
	} `xml:"owner"`
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
	return Workbook{LUID: item.ID, Name: item.Name, ContentURL: item.ContentURL, ProjectLUID: item.Project.ID, ProjectName: item.Project.Name, OwnerLUID: item.Owner.ID}
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

type fileUploadEnvelope struct {
	FileUpload struct {
		SessionID string `xml:"uploadSessionId,attr"`
	} `xml:"fileUpload"`
}

type publishEnvelope struct {
	Workbook struct {
		ID      string `xml:"id,attr"`
		Name    string `xml:"name,attr"`
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
	} `xml:"workbook"`
	Job jobXML `xml:"job"`
}

type jobEnvelope struct {
	Job jobXML `xml:"job"`
}

type validationEnvelope struct {
	Errors   []ValidationIssue `json:"errors"`
	Warnings []ValidationIssue `json:"warnings"`
}

type jobXML struct {
	ID         string `xml:"id,attr"`
	Type       string `xml:"type,attr"`
	Progress   *int   `xml:"progress,attr"`
	FinishCode *int   `xml:"finishCode,attr"`
}
