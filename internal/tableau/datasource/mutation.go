package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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
}
type PublishResult struct {
	Status, DatasourceLUID, DatasourceName, ProjectLUID, TableauRequestID string
	// ContentURL is the content URL Tableau assigned when its response reports one.
	ContentURL string
}
type MutationResult struct{ Status, DatasourceLUID, DatasourceName, ProjectLUID, OwnerLUID, TableauRequestID string }

// UpdateRequest contains only explicit datasource metadata changes.
type UpdateRequest struct {
	LUID        string
	Name        *string
	ProjectLUID *string
	OwnerLUID   *string
}

// PreparedPublish holds the publish snapshot until Commit or Close releases it.
type PreparedPublish interface {
	Commit(context.Context) (PublishResult, error)
	Close() error
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
	prepared := false
	defer func() {
		if !prepared {
			_ = snapshot.close()
		}
	}()
	payload, err := datasourcePublishPayload(input)
	if err != nil {
		return nil, err
	}
	parts := []tableau.MultipartPart{tableau.RequestPayloadPart(payload)}
	query := url.Values{}
	if input.Mode != PublishCreate {
		query.Set(string(input.Mode), "true")
	}
	if snapshot.size <= c.uploadThreshold {
		parts = append(parts, tableau.FilePart("tableau_datasource", input.Filename, snapshot.content, 0, snapshot.size))
	} else {
		session, err := c.uploadDatasource(ctx, input.Filename, snapshot)
		if err != nil {
			return nil, err
		}
		query.Set("uploadSessionId", session)
		query.Set("datasourceType", fileType)
	}
	body, err := tableau.NewMultipartBody(parts...)
	if err != nil {
		return nil, err
	}
	prepared = true
	return &preparedDatasourcePublish{client: c, query: query, body: body, release: snapshot.close, name: input.Name, project: input.ProjectLUID}, nil
}

// datasourcePublishSnapshot is an immutable, fingerprint-verified copy of the publish payload.
type datasourcePublishSnapshot struct {
	content *os.File
	size    int64
	close   func() error
}

// openDatasourcePublishSnapshot copies the managed artifact once into a temp snapshot, hashing it in
// the same pass, and verifies the snapshot against the planned fingerprint. All subsequent reads
// (streamed body or chunked upload) come from the snapshot, so the bytes published are exactly the
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
	return datasourcePublishSnapshot{content: snapshot, size: copied, close: cleanup}, nil
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
	client        *Client
	query         url.Values
	body          *tableau.MultipartBody
	release       func() error
	name, project string
	mu            sync.Mutex
	done          bool
}

// Commit sends the final publish request exactly once, then releases the
// publish snapshot. A snapshot cleanup failure never changes the publish result.
func (p *preparedDatasourcePublish) Commit(ctx context.Context) (PublishResult, error) {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return PublishResult{}, errors.New("prepared datasource publish was already committed or closed")
	}
	p.done = true
	p.mu.Unlock()
	defer func() { _ = p.release() }()
	response, err := p.client.transport.Do(ctx, p.client.session, tableau.Request{Method: http.MethodPost, ServerURL: p.client.serverURL, Path: p.client.sitePath("datasources"), Query: p.query, Multipart: p.body, Accept: "application/xml", Operation: "datasource.publish", MaxResponseBytes: maxListResponseBytes})
	if err != nil {
		if isUncertainDatasourceMutation(err) {
			return PublishResult{Status: "unknown", TableauRequestID: tableau.RequestID(err)}, err
		}
		return PublishResult{}, err
	}
	if response.StatusCode != http.StatusCreated {
		return PublishResult{Status: "unknown", TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.publish", response, fmt.Errorf("datasource publish returned HTTP %d, expected %d", response.StatusCode, http.StatusCreated), false)
	}
	var envelope datasourcePublishEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return PublishResult{Status: "unknown", TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.publish", response, fmt.Errorf("decode datasource publish response: %w", err), false)
	}
	ds := envelope.Datasource
	if ds.ID == "" || ds.Name != p.name || ds.Project.ID != p.project {
		return PublishResult{Status: "unknown", DatasourceLUID: ds.ID, DatasourceName: ds.Name, ProjectLUID: ds.Project.ID, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError("datasource.publish", response, errors.New("datasource publish response omitted or changed authoritative identity"), false)
	}
	return PublishResult{Status: "succeeded", DatasourceLUID: ds.ID, DatasourceName: ds.Name, ProjectLUID: ds.Project.ID, ContentURL: ds.ContentURL, TableauRequestID: response.TableauRequestID}, nil
}

// Close releases the publish snapshot when Commit never runs.
func (p *preparedDatasourcePublish) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return nil
	}
	p.done = true
	return p.release()
}

type datasourcePublishEnvelope struct {
	Datasource datasourceXML `xml:"datasource"`
}

func (c *Client) uploadDatasource(ctx context.Context, filename string, snapshot datasourcePublishSnapshot) (string, error) {
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
	for sequence, offset := 1, int64(0); offset < snapshot.size; sequence++ {
		blockSize := min(snapshot.size-offset, c.uploadChunkSize)
		body, err := tableau.NewMultipartBody(tableau.RequestPayloadPart(nil), tableau.FilePart("tableau_file", filename, snapshot.content, offset, blockSize))
		if err != nil {
			return "", err
		}
		offset += blockSize
		query := url.Values{}
		if ordered {
			query.Set("sequenceID", strconv.Itoa(sequence))
		}
		appended, err := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodPut, ServerURL: c.serverURL, Path: c.sitePath("fileUploads", initiated.Upload.ID), Query: query, Multipart: body, Accept: "application/xml", Operation: "datasource.upload.append", MaxResponseBytes: maxListResponseBytes})
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
	if input.Name == "" || input.ProjectLUID == "" || !tableau.ValidFilename(input.Filename) || input.ContentPath == "" || input.ContentSize <= 0 || input.ExpectedFingerprint == "" {
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

func datasourcePublishPayload(input PublishRequest) ([]byte, error) {
	type ds struct {
		Name    string `xml:"name,attr"`
		Project struct {
			ID string `xml:"id,attr"`
		} `xml:"project"`
		Parents []string `xml:"parentDataSourceUrls,omitempty"`
	}
	value := ds{Name: input.Name, Parents: input.ParentDataSourceURLs}
	value.Project.ID = input.ProjectLUID
	return xml.Marshal(struct {
		XMLName    xml.Name `xml:"tsRequest"`
		Datasource ds       `xml:"datasource"`
	}{Datasource: value})
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
