package datasource

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
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
)

type PublishMode string

const (
	PublishCreate    PublishMode = "create"
	PublishOverwrite PublishMode = "overwrite"
	PublishAppend    PublishMode = "append"
	PublishReplace   PublishMode = "replace"
)

type PublishRequest struct {
	Name, ProjectLUID, Filename, ContentPath, ExpectedFingerprint string
	ContentSize                                                   int64
	Mode                                                          PublishMode
	ParentDataSourceURLs                                          []string
	AsJob                                                         bool
	Accepted                                                      func(context.Context, string, string) (PublishResult, error)
}
type PublishResult struct{ Status, DatasourceLUID, DatasourceName, ProjectLUID, JobID, TableauRequestID, ReceiptPath string }
type MutationResult struct{ Status, DatasourceLUID, DatasourceName, ProjectLUID, OwnerLUID, TableauRequestID string }

// UpdateRequest contains only explicit datasource metadata changes.
type UpdateRequest struct {
	LUID        string
	Name        *string
	ProjectLUID *string
	OwnerLUID   *string
}
type PreparedPublish interface {
	Commit(context.Context) (PublishResult, error)
}

func (c *Client) Prepare(ctx context.Context, input PublishRequest) (PreparedPublish, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	fileType, err := validateMutationPublish(input)
	if err != nil {
		return nil, err
	}
	if len(input.ParentDataSourceURLs) > 0 {
		ok, err := apiAtLeast(c.transport.APIVersion(), 3, 29)
		if err != nil || !ok {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("composed datasource publish requires Tableau REST API 3.29 or later")
		}
	}
	if input.Mode == PublishReplace {
		ok, err := apiAtLeast(c.transport.APIVersion(), 3, 25)
		if err != nil || !ok {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("datasource replace requires Tableau REST API 3.25 or later")
		}
	}
	snapshot, err := openDatasourcePublishSnapshot(ctx, input)
	if err != nil {
		return nil, err
	}
	closed := false
	closeSnapshot := func() error {
		if closed {
			return nil
		}
		closed = true
		return snapshot.close()
	}
	defer func() { _ = closeSnapshot() }()
	query := url.Values{}
	if input.Mode != PublishCreate {
		query.Set(string(input.Mode), "true")
	}
	if input.AsJob {
		query.Set("asJob", "true")
	}
	var body []byte
	var contentType string
	if input.ContentSize <= c.uploadThreshold {
		content, err := io.ReadAll(snapshot.reader)
		if err != nil {
			return nil, errors.Join(err, closeSnapshot())
		}
		body, contentType, err = datasourcePublishBody(input, content, true)
		if err != nil {
			return nil, errors.Join(err, closeSnapshot())
		}
	} else {
		session, err := c.uploadDatasource(ctx, input.Filename, snapshot.reader, input.ContentSize)
		if err != nil {
			return nil, errors.Join(err, closeSnapshot())
		}
		query.Set("uploadSessionId", session)
		query.Set("datasourceType", fileType)
		body, contentType, err = datasourcePublishBody(input, nil, false)
		if err != nil {
			return nil, errors.Join(err, closeSnapshot())
		}
	}
	// The full request body is now materialized in memory, so the snapshot can be released.
	if err := closeSnapshot(); err != nil {
		return nil, fmt.Errorf("remove datasource publish snapshot: %w", err)
	}
	return &preparedDatasourcePublish{client: c, query: query, body: body, contentType: contentType, name: input.Name, project: input.ProjectLUID, asJob: input.AsJob, accepted: input.Accepted}, nil
}

// datasourcePublishSnapshot is an immutable, fingerprint-verified copy of the publish payload.
type datasourcePublishSnapshot struct {
	reader io.ReadSeeker
	size   int64
	close  func() error
}

// openDatasourcePublishSnapshot copies the managed artifact once into a temp snapshot, hashing it in
// the same pass, and verifies the snapshot against the planned fingerprint. All subsequent reads
// (inline body or chunked upload) come from the snapshot, so the bytes published are exactly the
// bytes fingerprinted, even if the source file is mutated after planning.
func openDatasourcePublishSnapshot(ctx context.Context, input PublishRequest) (datasourcePublishSnapshot, error) {
	source, err := os.Open(input.ContentPath)
	if err != nil {
		return datasourcePublishSnapshot{}, fmt.Errorf("open datasource publish payload: %w", err)
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != input.ContentSize {
		_ = source.Close()
		return datasourcePublishSnapshot{}, errors.New("datasource publish payload changed after planning")
	}
	snapshot, err := os.CreateTemp(filepath.Dir(input.ContentPath), ".tadx-datasource-publish-snapshot-*")
	if err != nil {
		_ = source.Close()
		return datasourcePublishSnapshot{}, fmt.Errorf("create datasource publish snapshot: %w", err)
	}
	cleanup := func() error {
		return errors.Join(snapshot.Close(), os.Remove(snapshot.Name()))
	}
	fingerprint, copied, err := fingerprintCopy(ctx, snapshot, source)
	sourceCloseErr := source.Close()
	if err != nil {
		return datasourcePublishSnapshot{}, errors.Join(fmt.Errorf("snapshot datasource publish payload: %w", err), sourceCloseErr, cleanup())
	}
	if sourceCloseErr != nil {
		return datasourcePublishSnapshot{}, errors.Join(fmt.Errorf("close datasource publish payload: %w", sourceCloseErr), cleanup())
	}
	if copied != input.ContentSize || fingerprint != input.ExpectedFingerprint {
		return datasourcePublishSnapshot{}, errors.Join(errors.New("datasource publish payload changed after planning"), cleanup())
	}
	if _, err := snapshot.Seek(0, io.SeekStart); err != nil {
		return datasourcePublishSnapshot{}, errors.Join(fmt.Errorf("rewind datasource publish snapshot: %w", err), cleanup())
	}
	return datasourcePublishSnapshot{reader: snapshot, size: copied, close: cleanup}, nil
}

// fingerprintCopy streams source into destination once, returning the sha256 fingerprint and byte count.
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

func (c *Client) Delete(ctx context.Context, luid string) (MutationResult, error) {
	if strings.TrimSpace(luid) == "" {
		return MutationResult{}, errors.New("datasource delete requires an exact LUID")
	}
	if err := c.validate(); err != nil {
		return MutationResult{}, err
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodDelete, ServerURL: c.serverURL, Path: c.sitePath("datasources", luid), Accept: "application/xml", Operation: "datasource.delete", MaxResponseBytes: maxListResponseBytes})
	if err != nil {
		if isUncertainDatasourceMutation(err) {
			return MutationResult{Status: "unknown", DatasourceLUID: luid, TableauRequestID: tableau.RequestID(err)}, err
		}
		return MutationResult{}, err
	}
	if response.StatusCode != http.StatusNoContent || len(response.Body) != 0 {
		return MutationResult{Status: "unknown", DatasourceLUID: luid, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.delete", response, fmt.Errorf("datasource delete returned HTTP %d with %d response bytes, expected empty HTTP 204", response.StatusCode, len(response.Body)), false)
	}
	return MutationResult{Status: "succeeded", DatasourceLUID: luid, TableauRequestID: response.TableauRequestID}, nil
}

// Update changes only explicit fields on one exact datasource.
func (c *Client) Update(ctx context.Context, input UpdateRequest) (MutationResult, error) {
	if err := c.validate(); err != nil {
		return MutationResult{}, err
	}
	input.LUID = strings.TrimSpace(input.LUID)
	if input.LUID == "" {
		return MutationResult{}, errors.New("datasource update requires an exact LUID")
	}
	if input.Name == nil && input.ProjectLUID == nil && input.OwnerLUID == nil {
		return MutationResult{}, errors.New("datasource update requires at least one explicit field")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return MutationResult{}, errors.New("datasource update name cannot be empty")
	}
	if input.ProjectLUID != nil && strings.TrimSpace(*input.ProjectLUID) == "" {
		return MutationResult{}, errors.New("datasource update project LUID cannot be empty")
	}
	if input.OwnerLUID != nil && strings.TrimSpace(*input.OwnerLUID) == "" {
		return MutationResult{}, errors.New("datasource update owner LUID cannot be empty")
	}
	if input.ProjectLUID != nil {
		value := strings.TrimSpace(*input.ProjectLUID)
		input.ProjectLUID = &value
	}
	if input.OwnerLUID != nil {
		value := strings.TrimSpace(*input.OwnerLUID)
		input.OwnerLUID = &value
	}
	body, err := xml.Marshal(datasourceUpdateEnvelope{Datasource: datasourceUpdateXML{Name: input.Name, Project: optionalDatasourceLUID(input.ProjectLUID), Owner: optionalDatasourceLUID(input.OwnerLUID)}})
	if err != nil {
		return MutationResult{}, fmt.Errorf("encode datasource update request: %w", err)
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodPut, ServerURL: c.serverURL, Path: c.sitePath("datasources", input.LUID), Body: body, ContentType: "application/xml", Accept: "application/xml", Operation: "datasource.update", MaxResponseBytes: maxListResponseBytes})
	if err != nil {
		if isUncertainDatasourceMutation(err) {
			return MutationResult{Status: "unknown", DatasourceLUID: input.LUID, TableauRequestID: tableau.RequestID(err)}, err
		}
		return MutationResult{}, err
	}
	if response.StatusCode != http.StatusOK {
		return MutationResult{Status: "unknown", DatasourceLUID: input.LUID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.update", response, fmt.Errorf("datasource update returned HTTP %d, expected 200", response.StatusCode), false)
	}
	var envelope datasourceGetEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return MutationResult{Status: "unknown", DatasourceLUID: input.LUID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.update", response, fmt.Errorf("decode datasource update response: %w", err), false)
	}
	datasource := normalizeDatasource(envelope.Datasource)
	if datasource.LUID != input.LUID || datasource.Name == "" || datasource.ProjectLUID == "" || datasource.OwnerLUID == "" {
		return MutationResult{Status: "unknown", DatasourceLUID: input.LUID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.update", response, errors.New("datasource update response omitted or changed authoritative identity"), false)
	}
	if (input.Name != nil && datasource.Name != *input.Name) || (input.ProjectLUID != nil && datasource.ProjectLUID != *input.ProjectLUID) || (input.OwnerLUID != nil && datasource.OwnerLUID != *input.OwnerLUID) {
		return MutationResult{Status: "unknown", DatasourceLUID: datasource.LUID, DatasourceName: datasource.Name, ProjectLUID: datasource.ProjectLUID, OwnerLUID: datasource.OwnerLUID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.update", response, errors.New("datasource update response did not preserve the requested changes"), false)
	}
	return MutationResult{Status: "succeeded", DatasourceLUID: datasource.LUID, DatasourceName: datasource.Name, ProjectLUID: datasource.ProjectLUID, OwnerLUID: datasource.OwnerLUID, TableauRequestID: response.TableauRequestID}, nil
}

type datasourceUpdateEnvelope struct {
	XMLName    xml.Name            `xml:"tsRequest"`
	Datasource datasourceUpdateXML `xml:"datasource"`
}

type datasourceUpdateXML struct {
	Name    *string            `xml:"name,attr,omitempty"`
	Project *datasourceLUIDXML `xml:"project,omitempty"`
	Owner   *datasourceLUIDXML `xml:"owner,omitempty"`
}

type datasourceLUIDXML struct {
	ID string `xml:"id,attr"`
}

func optionalDatasourceLUID(value *string) *datasourceLUIDXML {
	if value == nil {
		return nil
	}
	return &datasourceLUIDXML{ID: strings.TrimSpace(*value)}
}

type preparedDatasourcePublish struct {
	client                     *Client
	query                      url.Values
	body                       []byte
	contentType, name, project string
	mu                         sync.Mutex
	committed                  bool
	asJob                      bool
	accepted                   func(context.Context, string, string) (PublishResult, error)
}

func (p *preparedDatasourcePublish) Commit(ctx context.Context) (PublishResult, error) {
	p.mu.Lock()
	if p.committed {
		p.mu.Unlock()
		return PublishResult{}, errors.New("prepared datasource publish was already committed")
	}
	p.committed = true
	p.mu.Unlock()
	response, err := p.client.transport.Do(ctx, p.client.session, tableau.Request{Method: http.MethodPost, ServerURL: p.client.serverURL, Path: p.client.sitePath("datasources"), Query: p.query, Body: p.body, ContentType: p.contentType, Accept: "application/xml", Operation: "datasource.publish", MaxResponseBytes: maxListResponseBytes})
	if err != nil {
		if isUncertainDatasourceMutation(err) {
			return PublishResult{Status: "unknown", TableauRequestID: tableau.RequestID(err)}, err
		}
		return PublishResult{}, err
	}
	expectedStatus := http.StatusCreated
	if p.asJob {
		expectedStatus = http.StatusAccepted
	}
	if response.StatusCode != expectedStatus {
		return PublishResult{Status: "unknown", TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.publish", response, fmt.Errorf("datasource publish returned HTTP %d, expected %d", response.StatusCode, expectedStatus), false)
	}
	var envelope datasourcePublishEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return PublishResult{Status: "unknown", JobID: envelope.Job.ID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.publish", response, fmt.Errorf("decode datasource publish response: %w", err), false)
	}
	if p.asJob {
		if err := validateAcceptedDatasourceJob(envelope.Job); err != nil {
			return PublishResult{Status: "unknown", JobID: envelope.Job.ID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.publish", response, err, false)
		}
		if p.accepted != nil {
			return p.accepted(ctx, envelope.Job.ID, response.TableauRequestID)
		}
		return p.client.pollDatasourceJob(ctx, envelope.Job.ID, response.TableauRequestID, p.name, p.project)
	}
	ds := envelope.Datasource
	if ds.ID == "" || ds.Name != p.name || ds.Project.ID != p.project {
		return PublishResult{Status: "unknown", DatasourceLUID: ds.ID, DatasourceName: ds.Name, ProjectLUID: ds.Project.ID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.publish", response, errors.New("datasource publish response omitted or changed authoritative identity"), false)
	}
	return PublishResult{Status: "succeeded", DatasourceLUID: ds.ID, DatasourceName: ds.Name, ProjectLUID: ds.Project.ID, TableauRequestID: response.TableauRequestID}, nil
}

func validateAcceptedDatasourceJob(job datasourceJobXML) error {
	if strings.TrimSpace(job.ID) == "" {
		return errors.New("asynchronous datasource publish response omitted job identity")
	}
	if job.Type != "PublishDatasource" {
		return fmt.Errorf("asynchronous datasource publish returned job type %q, expected %q", job.Type, "PublishDatasource")
	}
	if job.Progress == nil || job.FinishCode == nil || *job.Progress != 0 || *job.FinishCode != 1 {
		return errors.New("asynchronous datasource publish returned an invalid accepted job state")
	}
	return nil
}

func (c *Client) pollDatasourceJob(ctx context.Context, jobID, acceptedRequestID, expectedName, expectedProject string) (PublishResult, error) {
	pollCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()
	lastRequestID := acceptedRequestID
	for {
		response, err := c.transport.Do(pollCtx, c.session, tableau.Request{Method: http.MethodGet, ServerURL: c.serverURL, Path: c.sitePath("jobs", jobID), Accept: "application/xml", Operation: "datasource.publish.poll", MaxResponseBytes: maxListResponseBytes})
		if err != nil {
			requestID := tableau.RequestID(err)
			if requestID == "" {
				requestID = lastRequestID
			}
			if ctx.Err() != nil {
				return PublishResult{Status: "cancelled", JobID: jobID, TableauRequestID: requestID}, ctx.Err()
			}
			if errors.Is(pollCtx.Err(), context.DeadlineExceeded) {
				return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau datasource publish job %s timed out after %s: %w", jobID, c.pollTimeout, err)
			}
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
					return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau datasource publish job %s timed out after %s: %w", jobID, c.pollTimeout, err)
				case <-time.After(delay):
				}
				continue
			}
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, err
		}
		if response.TableauRequestID != "" {
			lastRequestID = response.TableauRequestID
		}
		requestID := lastRequestID
		response.TableauRequestID = requestID
		var envelope datasourceJobEnvelope
		if err := xml.Unmarshal(response.Body, &envelope); err != nil {
			if ctx.Err() != nil {
				return PublishResult{Status: "cancelled", JobID: jobID, TableauRequestID: requestID}, ctx.Err()
			}
			if errors.Is(pollCtx.Err(), context.DeadlineExceeded) {
				return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau datasource publish job %s timed out after %s: %w", jobID, c.pollTimeout, err)
			}
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, fmt.Errorf("decode Tableau job response: %w", err), true)
		}
		if envelope.Job.ID != jobID {
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, fmt.Errorf("Tableau job response returned job ID %q, expected %q", envelope.Job.ID, jobID), true)
		}
		if envelope.Job.Type != "PublishDatasource" {
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, fmt.Errorf("Tableau job response returned type %q, expected %q", envelope.Job.Type, "PublishDatasource"), true)
		}
		if envelope.Job.Progress == nil || envelope.Job.FinishCode == nil {
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, errors.New("Tableau job response omitted progress or finish code"), true)
		}
		progress, finishCode := *envelope.Job.Progress, *envelope.Job.FinishCode
		switch {
		case progress == 0 && finishCode == 1:
		case progress == 100 && finishCode == 0:
			if envelope.Job.DatasourceCount == 0 {
				return PublishResult{Status: "succeeded", JobID: jobID, TableauRequestID: requestID}, nil
			}
			if envelope.Job.DatasourceCount != 1 || envelope.Job.Datasource == nil || strings.TrimSpace(envelope.Job.Datasource.ID) == "" {
				return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, errors.New("successful Tableau datasource publish job omitted one authoritative datasource identity"), false)
			}
			datasource := envelope.Job.Datasource
			if datasource.Name != "" && datasource.Name != expectedName {
				return PublishResult{Status: "unknown", DatasourceLUID: datasource.ID, DatasourceName: datasource.Name, ProjectLUID: datasource.Project.ID, JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, fmt.Errorf("successful Tableau datasource publish job returned datasource name %q, expected %q", datasource.Name, expectedName), false)
			}
			if datasource.Project.ID != "" && datasource.Project.ID != expectedProject {
				return PublishResult{Status: "unknown", DatasourceLUID: datasource.ID, DatasourceName: datasource.Name, ProjectLUID: datasource.Project.ID, JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, fmt.Errorf("successful Tableau datasource publish job returned project LUID %q, expected %q", datasource.Project.ID, expectedProject), false)
			}
			return PublishResult{Status: "succeeded", DatasourceLUID: datasource.ID, DatasourceName: expectedName, ProjectLUID: expectedProject, JobID: jobID, TableauRequestID: requestID}, nil
		case progress == 100 && (finishCode == 1 || finishCode == 2):
			return PublishResult{Status: "failed", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau datasource publish job %s failed with finish code %d", jobID, finishCode)
		case progress == 100:
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, fmt.Errorf("Tableau job response returned unknown finish code %d", finishCode), true)
		default:
			return PublishResult{Status: "unknown", JobID: jobID, TableauRequestID: requestID}, tableau.NewProtocolError("datasource.publish.poll", response, fmt.Errorf("Tableau job response returned invalid state progress=%d finishCode=%d", progress, finishCode), true)
		}
		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return PublishResult{Status: "cancelled", JobID: jobID, TableauRequestID: requestID}, ctx.Err()
			}
			return PublishResult{Status: "timed_out", JobID: jobID, TableauRequestID: requestID}, fmt.Errorf("Tableau datasource publish job %s timed out after %s", jobID, c.pollTimeout)
		case <-time.After(c.pollInterval):
		}
	}
}

type datasourcePublishEnvelope struct {
	Datasource datasourceXML    `xml:"datasource"`
	Job        datasourceJobXML `xml:"job"`
}

type datasourceJobEnvelope struct {
	Job datasourceJobXML `xml:"job"`
}

type datasourceJobXML struct {
	ID              string
	Type            string
	Progress        *int
	FinishCode      *int
	Datasource      *datasourceXML
	DatasourceCount int
}

func (j *datasourceJobXML) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	for _, attribute := range start.Attr {
		switch attribute.Name.Local {
		case "id":
			j.ID = attribute.Value
		case "type":
			j.Type = attribute.Value
		case "progress":
			value, err := strconv.Atoi(attribute.Value)
			if err != nil {
				return fmt.Errorf("decode datasource job progress: %w", err)
			}
			j.Progress = &value
		case "finishCode":
			value, err := strconv.Atoi(attribute.Value)
			if err != nil {
				return fmt.Errorf("decode datasource job finish code: %w", err)
			}
			j.FinishCode = &value
		}
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local != "datasource" {
				continue
			}
			var datasource datasourceXML
			if err := decoder.DecodeElement(&datasource, &value); err != nil {
				return err
			}
			j.DatasourceCount++
			if j.DatasourceCount == 1 {
				j.Datasource = &datasource
			}
		case xml.EndElement:
			if value.Name == start.Name {
				return nil
			}
		}
	}
}

func (c *Client) uploadDatasource(ctx context.Context, filename string, source io.Reader, size int64) (string, error) {
	ordered, err := apiAtLeast(c.transport.APIVersion(), 3, 27)
	if err != nil {
		return "", err
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodPost, ServerURL: c.serverURL, Path: c.sitePath("fileUploads"), Accept: "application/xml", Operation: "datasource.upload.initiate", MaxResponseBytes: maxListResponseBytes})
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusCreated {
		return "", tableau.NewProtocolError("datasource.upload.initiate", response, errors.New("initiate upload did not return HTTP 201"), false)
	}
	var initiated struct {
		Upload struct {
			ID string `xml:"uploadSessionId,attr"`
		} `xml:"fileUpload"`
	}
	if err := xml.Unmarshal(response.Body, &initiated); err != nil || initiated.Upload.ID == "" {
		return "", tableau.NewProtocolError("datasource.upload.initiate", response, errors.New("initiate upload omitted session ID"), false)
	}
	remaining := size
	for sequence := 1; remaining > 0; sequence++ {
		blockSize := min(remaining, c.uploadChunkSize)
		block := make([]byte, blockSize)
		if _, err := io.ReadFull(source, block); err != nil {
			return "", err
		}
		remaining -= blockSize
		body, contentType, err := uploadAppendBody(filename, block)
		if err != nil {
			return "", err
		}
		query := url.Values{}
		if ordered {
			query.Set("sequenceID", strconv.Itoa(sequence))
		}
		appended, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodPut, ServerURL: c.serverURL, Path: c.sitePath("fileUploads", initiated.Upload.ID), Query: query, Body: body, ContentType: contentType, Accept: "application/xml", Operation: "datasource.upload.append", MaxResponseBytes: maxListResponseBytes})
		if err != nil {
			return "", err
		}
		if appended.StatusCode != http.StatusOK {
			return "", tableau.NewProtocolError("datasource.upload.append", appended, errors.New("append upload did not return HTTP 200"), false)
		}
		var confirm struct {
			Upload struct {
				ID string `xml:"uploadSessionId,attr"`
			} `xml:"fileUpload"`
		}
		if xml.Unmarshal(appended.Body, &confirm) != nil || confirm.Upload.ID != initiated.Upload.ID {
			return "", tableau.NewProtocolError("datasource.upload.append", appended, errors.New("append upload changed session identity"), false)
		}
	}
	return initiated.Upload.ID, nil
}

func validateMutationPublish(input PublishRequest) (string, error) {
	if input.Name == "" || input.ProjectLUID == "" || !validFilename(input.Filename) || input.ContentPath == "" || input.ContentSize <= 0 || input.ExpectedFingerprint == "" {
		return "", errors.New("datasource publish requires authoritative identity and a planned native payload")
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.Filename)), ".")
	if extension != "tds" && extension != "tdsx" && extension != "hyper" && extension != "tde" {
		return "", fmt.Errorf("unsupported datasource type %q", extension)
	}
	if input.Mode != PublishCreate && input.Mode != PublishOverwrite && input.Mode != PublishAppend && input.Mode != PublishReplace {
		return "", errors.New("datasource publish requires an explicit mode")
	}
	return extension, nil
}

func datasourcePublishBody(input PublishRequest, content []byte, include bool) ([]byte, string, error) {
	type ds struct {
		Name    string `xml:"name,attr"`
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
		Parents []string `xml:"parentDataSourceUrls,omitempty"`
	}
	value := ds{Name: input.Name, Parents: input.ParentDataSourceURLs}
	value.Project.ID = input.ProjectLUID
	payload, err := xml.Marshal(struct {
		XMLName    xml.Name `xml:"tsRequest"`
		Datasource ds       `xml:"datasource"`
	}{Datasource: value})
	if err != nil {
		return nil, "", err
	}
	return mixedBody(payload, input.Filename, content, include, "tableau_datasource")
}
func uploadAppendBody(filename string, content []byte) ([]byte, string, error) {
	return mixedBody(nil, filename, content, true, "tableau_file")
}
func mixedBody(payload []byte, filename string, content []byte, include bool, partName string) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	request, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`name="request_payload"`}, "Content-Type": {"text/xml"}})
	if err != nil {
		return nil, "", err
	}
	if _, err := request.Write(payload); err != nil {
		return nil, "", err
	}
	if include {
		disposition := mime.FormatMediaType("form-data", map[string]string{"filename": filepath.Base(filename)})
		part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {fmt.Sprintf(`name="%s"; %s`, partName, strings.TrimPrefix(disposition, "form-data; "))}, "Content-Type": {"application/octet-stream"}})
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(content); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "multipart/mixed; boundary=" + writer.Boundary(), nil
}
func apiAtLeast(value string, major, minor int) (bool, error) {
	parts := strings.SplitN(value, ".", 3)
	if len(parts) < 2 {
		return false, fmt.Errorf("malformed Tableau REST API version %q", value)
	}
	a, e1 := strconv.Atoi(parts[0])
	b, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil {
		return false, fmt.Errorf("malformed Tableau REST API version %q", value)
	}
	return a > major || (a == major && b >= minor), nil
}

func isUncertainDatasourceMutation(err error) bool {
	var status interface{ HTTPStatus() int }
	return !errors.As(err, &status) || (status.HTTPStatus() >= http.StatusOK && status.HTTPStatus() < http.StatusMultipleChoices)
}
