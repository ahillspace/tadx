package datasource

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/commitguard"
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

// PreparedPublish holds the publish snapshot until Commit sends and releases it.
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
		ok, err := tableau.APIVersionAtLeast(c.transport.APIVersion(), 3, 29)
		if err != nil || !ok {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("composed datasource publish requires Tableau REST API 3.29 or later")
		}
	}
	if input.Mode == PublishReplace {
		ok, err := tableau.APIVersionAtLeast(c.transport.APIVersion(), 3, 25)
		if err != nil || !ok {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("datasource replace requires Tableau REST API 3.25 or later")
		}
	}
	snapshot, err := tableau.SnapshotPublishPayload(ctx, input.ContentPath, input.ContentSize, input.ExpectedFingerprint)
	if err != nil {
		return nil, fmt.Errorf("prepare datasource publish payload: %w", err)
	}
	prepared := false
	defer func() {
		if !prepared {
			_ = snapshot.Close()
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
	if snapshot.Size() <= c.uploadThreshold {
		parts = append(parts, tableau.FilePart("tableau_datasource", input.Filename, snapshot.File(), 0, snapshot.Size()))
	} else {
		upload := tableau.FileUpload{Transport: c.transport, Session: c.session, ServerURL: c.serverURL, Operation: "datasource", Filename: input.Filename, ChunkSize: c.uploadChunkSize}
		session, err := upload.Upload(ctx, snapshot)
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
	return &preparedDatasourcePublish{client: c, query: query, body: body, snapshot: snapshot, name: input.Name, project: input.ProjectLUID}, nil
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
	snapshot      *tableau.PublishSnapshot
	name, project string
}

// Commit sends the final publish request exactly once, then releases the publish
// snapshot. A snapshot cleanup failure never changes the publish result.
func (p *preparedDatasourcePublish) Commit(ctx context.Context) (PublishResult, error) {
	ctx, err := commitguard.Admit(ctx)
	if err != nil {
		_ = p.snapshot.Close()
		return PublishResult{}, err
	}
	if err := p.snapshot.Claim(); err != nil {
		return PublishResult{}, fmt.Errorf("prepared datasource publish: %w", err)
	}
	defer func() { _ = p.snapshot.Close() }()
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

type datasourcePublishEnvelope struct {
	Datasource datasourceXML `xml:"datasource"`
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

func isUncertainDatasourceMutation(err error) bool {
	var status interface{ HTTPStatus() int }
	return !errors.As(err, &status) || (status.HTTPStatus() >= http.StatusOK && status.HTTPStatus() < http.StatusMultipleChoices)
}
