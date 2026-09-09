package project

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	maxPageSize      = 1000
	maxResponseBytes = 16 * 1024 * 1024
	listOperation    = "project.list"
	createOperation  = "project.create"
	updateOperation  = "project.update"
	deleteOperation  = "project.delete"
)

// Client is an authenticated project REST client.
type Client struct {
	transport *tableau.Transport
	session   auth.Session
	serverURL string
}

// NewClient creates an authenticated project REST client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{transport: transport, session: session, serverURL: serverURL}
}

// List returns one bounded classic REST project page.
func (c *Client) List(ctx context.Context, input ListRequest) (Page, error) {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return Page{}, errors.New("Tableau project client is not configured")
	}
	query, err := listQuery(input)
	if err != nil {
		return Page{}, err
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method:           http.MethodGet,
		ServerURL:        c.serverURL,
		Path:             c.sitePath("projects"),
		Query:            query,
		Accept:           "application/xml",
		Operation:        listOperation,
		MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		return Page{}, err
	}

	var envelope listEnvelopeXML
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Page{}, tableau.NewProtocolError(listOperation, response, fmt.Errorf("decode project list response: %w", err), true)
	}
	if len(envelope.Pagination) != 1 {
		return Page{}, tableau.NewProtocolError(listOperation, response, fmt.Errorf("project list response contained %d pagination elements; expected 1", len(envelope.Pagination)), true)
	}
	if len(envelope.Projects) != 1 {
		return Page{}, tableau.NewProtocolError(listOperation, response, fmt.Errorf("project list response contained %d projects elements; expected 1", len(envelope.Projects)), true)
	}

	itemsXML := envelope.Projects[0].Items
	page, err := normalizePagination(envelope.Pagination[0], input.PageNumber, input.PageSize, len(itemsXML))
	if err != nil {
		return Page{}, tableau.NewProtocolError(listOperation, response, err, true)
	}
	items := make([]Project, len(itemsXML))
	seen := make(map[string]Project, len(itemsXML))
	for index, item := range itemsXML {
		project := normalizeProject(item)
		if strings.TrimSpace(project.LUID) == "" || strings.TrimSpace(project.Name) == "" {
			return Page{}, tableau.NewProtocolError(listOperation, response, fmt.Errorf("project list response returned an incomplete authoritative identity at item %d", index), true)
		}
		if previous, exists := seen[project.LUID]; exists && !reflect.DeepEqual(previous, project) {
			return Page{}, tableau.NewProtocolError(listOperation, response, fmt.Errorf("project list response returned conflicting records for LUID %q", project.LUID), true)
		}
		seen[project.LUID] = project
		items[index] = project
	}
	return Page{Number: page.Number, Size: page.Size, Total: page.Total, Items: items, TableauRequestID: response.TableauRequestID}, nil
}

// Create creates one project under an optional exact parent.
func (c *Client) Create(ctx context.Context, input CreateRequest) (MutationResult, error) {
	if err := c.validateMutationClient(); err != nil {
		return MutationResult{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return MutationResult{}, errors.New("project create requires a name")
	}
	if strings.Contains(input.Name, "/") {
		return MutationResult{}, errors.New("project create name cannot contain a slash")
	}
	if err := validateContentPermissions(input.ContentPermissions); err != nil {
		return MutationResult{}, err
	}
	body, err := xml.Marshal(createEnvelopeXML{Project: createProjectXML{
		Name: input.Name, Description: input.Description, ParentLUID: input.ParentLUID, ContentPermissions: input.ContentPermissions,
	}})
	if err != nil {
		return MutationResult{}, fmt.Errorf("encode project create request: %w", err)
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodPost, ServerURL: c.serverURL, Path: c.sitePath("projects"), Body: body,
		Accept: "application/xml", ContentType: "application/xml", Operation: createOperation, MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		return MutationResult{Status: "unknown", Project: Project{Name: input.Name, ParentLUID: input.ParentLUID}, TableauRequestID: tableau.RequestID(err)}, err
	}
	if response.StatusCode != http.StatusCreated {
		return MutationResult{}, tableau.NewProtocolError(createOperation, response, fmt.Errorf("project create returned HTTP %d, expected 201", response.StatusCode), false)
	}
	project, err := decodeMutationProject(response, createOperation)
	if err != nil {
		return MutationResult{}, err
	}
	if project.Name != input.Name || project.ParentLUID != input.ParentLUID {
		return MutationResult{}, tableau.NewProtocolError(createOperation, response, errors.New("project create response changed the requested name or parent identity"), false)
	}
	if input.Description != "" && project.Description != input.Description {
		return MutationResult{}, tableau.NewProtocolError(createOperation, response, errors.New("project create response changed the requested description"), false)
	}
	if input.ContentPermissions != "" && project.ContentPermissions != input.ContentPermissions {
		return MutationResult{}, tableau.NewProtocolError(createOperation, response, errors.New("project create response changed the requested content permissions"), false)
	}
	return MutationResult{Status: "succeeded", Project: project, TableauRequestID: response.TableauRequestID}, nil
}

// Update updates only explicit bounded project metadata fields.
func (c *Client) Update(ctx context.Context, input UpdateRequest) (MutationResult, error) {
	if err := c.validateMutationClient(); err != nil {
		return MutationResult{}, err
	}
	if strings.TrimSpace(input.LUID) == "" {
		return MutationResult{}, errors.New("project update requires an exact project LUID")
	}
	if input.Name == nil && input.Description == nil && input.ContentPermissions == nil && input.ParentLUID == nil {
		return MutationResult{}, errors.New("project update requires at least one explicit metadata field")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return MutationResult{}, errors.New("project update name cannot be empty")
	}
	if input.Name != nil && strings.Contains(*input.Name, "/") {
		return MutationResult{}, errors.New("project update name cannot contain a slash")
	}
	if input.ContentPermissions != nil {
		if err := validateContentPermissions(*input.ContentPermissions); err != nil {
			return MutationResult{}, err
		}
	}
	body, err := xml.Marshal(updateEnvelopeXML{Project: updateProjectXML{Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions, ParentLUID: input.ParentLUID}})
	if err != nil {
		return MutationResult{}, fmt.Errorf("encode project update request: %w", err)
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodPut, ServerURL: c.serverURL, Path: c.sitePath("projects", input.LUID), Body: body,
		Accept: "application/xml", ContentType: "application/xml", Operation: updateOperation, MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		return MutationResult{Status: "unknown", Project: Project{LUID: input.LUID}, TableauRequestID: tableau.RequestID(err)}, err
	}
	if response.StatusCode != http.StatusOK {
		return MutationResult{}, tableau.NewProtocolError(updateOperation, response, fmt.Errorf("project update returned HTTP %d, expected 200", response.StatusCode), false)
	}
	project, err := decodeMutationProject(response, updateOperation)
	if err != nil {
		return MutationResult{}, err
	}
	if project.LUID != input.LUID {
		return MutationResult{}, tableau.NewProtocolError(updateOperation, response, fmt.Errorf("project update response returned LUID %q, expected %q", project.LUID, input.LUID), false)
	}
	if input.Name != nil && project.Name != *input.Name {
		return MutationResult{}, tableau.NewProtocolError(updateOperation, response, errors.New("project update response changed the requested name"), false)
	}
	if input.Description != nil && project.Description != *input.Description {
		return MutationResult{}, tableau.NewProtocolError(updateOperation, response, errors.New("project update response changed the requested description"), false)
	}
	if input.ContentPermissions != nil && project.ContentPermissions != *input.ContentPermissions {
		return MutationResult{}, tableau.NewProtocolError(updateOperation, response, errors.New("project update response changed the requested content permissions"), false)
	}
	if input.ParentLUID != nil && project.ParentLUID != *input.ParentLUID {
		return MutationResult{}, tableau.NewProtocolError(updateOperation, response, errors.New("project update response changed the requested parent identity"), false)
	}
	return MutationResult{Status: "succeeded", Project: project, TableauRequestID: response.TableauRequestID}, nil
}

// Delete deletes one project by its authoritative LUID.
func (c *Client) Delete(ctx context.Context, luid string) (DeleteResult, error) {
	if err := c.validateMutationClient(); err != nil {
		return DeleteResult{}, err
	}
	luid = strings.TrimSpace(luid)
	if luid == "" {
		return DeleteResult{}, errors.New("project delete requires an exact project LUID")
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodDelete, ServerURL: c.serverURL, Path: c.sitePath("projects", luid),
		Operation: deleteOperation, MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		return DeleteResult{Status: "unknown", ProjectLUID: luid, TableauRequestID: tableau.RequestID(err)}, err
	}
	if response.StatusCode != http.StatusNoContent || len(response.Body) != 0 {
		return DeleteResult{Status: "unknown", ProjectLUID: luid, TableauRequestID: response.TableauRequestID}, tableau.NewProtocolError(deleteOperation, response, fmt.Errorf("project delete returned HTTP %d with %d response bytes, expected empty HTTP 204", response.StatusCode, len(response.Body)), false)
	}
	return DeleteResult{Status: "succeeded", ProjectLUID: luid, TableauRequestID: response.TableauRequestID}, nil
}

func (c *Client) validateMutationClient() error {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return errors.New("Tableau project client is not configured")
	}
	return nil
}

func validateContentPermissions(value string) error {
	if value == "" {
		return nil
	}
	switch value {
	case "ManagedByOwner", "LockedToProject", "LockedToProjectWithoutNested":
		return nil
	default:
		return fmt.Errorf("unsupported project content permissions %q", value)
	}
}

func decodeMutationProject(response tableau.Response, operation string) (Project, error) {
	var envelope mutationEnvelopeXML
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Project{}, tableau.NewProtocolError(operation, response, fmt.Errorf("decode project mutation response: %w", err), false)
	}
	if len(envelope.Projects) != 1 {
		return Project{}, tableau.NewProtocolError(operation, response, fmt.Errorf("project mutation response contained %d project elements; expected 1", len(envelope.Projects)), false)
	}
	project := normalizeProject(envelope.Projects[0])
	if project.LUID == "" || project.Name == "" {
		return Project{}, tableau.NewProtocolError(operation, response, errors.New("project mutation response omitted authoritative identity"), false)
	}
	return project, nil
}

func (c *Client) sitePath(parts ...string) string {
	segments := []string{"api", c.transport.APIVersion(), "sites", c.session.SiteLUID()}
	segments = append(segments, parts...)
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return "/" + strings.Join(segments, "/")
}

func listQuery(input ListRequest) (url.Values, error) {
	if input.PageNumber <= 0 {
		return nil, errors.New("project page number must be positive")
	}
	if input.PageSize <= 0 || input.PageSize > maxPageSize {
		return nil, fmt.Errorf("project page size must be between 1 and %d", maxPageSize)
	}
	query := url.Values{
		"pageNumber": {strconv.Itoa(input.PageNumber)},
		"pageSize":   {strconv.Itoa(input.PageSize)},
	}
	filter, err := ListFilter(input)
	if err != nil {
		return nil, err
	}
	if filter != "" {
		query.Set("filter", filter)
	}
	return query, nil
}

// ListFilter validates and encodes project selectors for paged and full lists.
// Pagination fields do not affect the selected population.
func ListFilter(input ListRequest) (string, error) {
	filters := make([]string, 0, 4)
	for _, filter := range []struct {
		field string
		value string
	}{
		{field: "name", value: input.Name},
		{field: "parentProjectId", value: input.ParentLUID},
		{field: "ownerName", value: input.OwnerName},
	} {
		if filter.value == "" {
			continue
		}
		if strings.ContainsAny(filter.value, ",&") {
			return "", fmt.Errorf("project %s filter contains an unsupported comma or ampersand", filter.field)
		}
		filters = append(filters, filter.field+":eq:"+filter.value)
	}
	if input.TopLevel != nil {
		filters = append(filters, "topLevelProject:eq:"+strconv.FormatBool(*input.TopLevel))
	}
	return strings.Join(filters, ","), nil
}

type listEnvelopeXML struct {
	XMLName    xml.Name         `xml:"tsResponse"`
	Pagination []paginationXML  `xml:"pagination"`
	Projects   []projectListXML `xml:"projects"`
}

type createEnvelopeXML struct {
	XMLName xml.Name         `xml:"tsRequest"`
	Project createProjectXML `xml:"project"`
}

type createProjectXML struct {
	Name               string `xml:"name,attr"`
	Description        string `xml:"description,attr,omitempty"`
	ParentLUID         string `xml:"parentProjectId,attr,omitempty"`
	ContentPermissions string `xml:"contentPermissions,attr,omitempty"`
}

type updateEnvelopeXML struct {
	XMLName xml.Name         `xml:"tsRequest"`
	Project updateProjectXML `xml:"project"`
}

type updateProjectXML struct {
	Name               *string `xml:"name,attr,omitempty"`
	Description        *string `xml:"description,attr,omitempty"`
	ContentPermissions *string `xml:"contentPermissions,attr,omitempty"`
	ParentLUID         *string `xml:"parentProjectId,attr,omitempty"`
}

type mutationEnvelopeXML struct {
	XMLName  xml.Name     `xml:"tsResponse"`
	Projects []projectXML `xml:"project"`
}

type paginationXML struct {
	Number *int `xml:"pageNumber,attr"`
	Size   *int `xml:"pageSize,attr"`
	Total  *int `xml:"totalAvailable,attr"`
}

type projectListXML struct {
	Items []projectXML `xml:"project"`
}

type projectXML struct {
	ID                              string `xml:"id,attr"`
	Name                            string `xml:"name,attr"`
	Description                     string `xml:"description,attr"`
	ParentProjectID                 string `xml:"parentProjectId,attr"`
	TopLevel                        *bool  `xml:"topLevelProject,attr"`
	ContentPermissions              string `xml:"contentPermissions,attr"`
	ControllingPermissionsProjectID string `xml:"controllingPermissionsProjectId,attr"`
	CreatedAt                       string `xml:"createdAt,attr"`
	UpdatedAt                       string `xml:"updatedAt,attr"`
	Owner                           *struct {
		ID string `xml:"id,attr"`
	} `xml:"owner"`
	ContentCounts *struct {
		ProjectCount    *int `xml:"projectCount,attr"`
		WorkbookCount   *int `xml:"workbookCount,attr"`
		ViewCount       *int `xml:"viewCount,attr"`
		DatasourceCount *int `xml:"datasourceCount,attr"`
	} `xml:"contentCounts"`
}

func normalizeProject(item projectXML) Project {
	project := Project{
		LUID: strings.TrimSpace(item.ID), Name: strings.TrimSpace(item.Name), Description: item.Description, ParentLUID: strings.TrimSpace(item.ParentProjectID),
		TopLevel: item.TopLevel, ContentPermissions: item.ContentPermissions,
		ControllingPermissionsProjectID: item.ControllingPermissionsProjectID,
		CreatedAt:                       item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
	if item.Owner != nil {
		project.OwnerLUID = strings.TrimSpace(item.Owner.ID)
	}
	if item.ContentCounts != nil {
		project.ProjectCount = item.ContentCounts.ProjectCount
		project.WorkbookCount = item.ContentCounts.WorkbookCount
		project.ViewCount = item.ContentCounts.ViewCount
		project.DatasourceCount = item.ContentCounts.DatasourceCount
	}
	return project
}

func normalizePagination(value paginationXML, requestedNumber, requestedSize, itemCount int) (tableau.Page, error) {
	if value.Number == nil || value.Size == nil || value.Total == nil {
		return tableau.Page{}, errors.New("project list response omitted required pagination attributes")
	}
	number, size, total := *value.Number, *value.Size, *value.Total
	if number != requestedNumber || number <= 0 {
		return tableau.Page{}, fmt.Errorf("project list response returned page number %d, expected %d", number, requestedNumber)
	}
	if size <= 0 || size > requestedSize || size > maxPageSize {
		return tableau.Page{}, fmt.Errorf("project list response returned invalid page size %d", size)
	}
	if total < 0 || itemCount > size || total < itemCount {
		return tableau.Page{}, fmt.Errorf("project list response returned inconsistent pagination total %d, size %d, and item count %d", total, size, itemCount)
	}
	pageIndex := int64(number - 1)
	if pageIndex > math.MaxInt64/int64(size) {
		return tableau.Page{}, fmt.Errorf("project list response page %d exceeds the pagination bound", number)
	}
	offset := pageIndex * int64(size)
	remaining := int64(total) - offset
	if remaining < 0 {
		return tableau.Page{}, fmt.Errorf("project list response total %d is inconsistent with page %d", total, number)
	}
	expected := min(int64(size), remaining)
	if int64(itemCount) != expected {
		return tableau.Page{}, fmt.Errorf("project list response returned %d items for page %d; expected %d from total %d and size %d", itemCount, number, expected, total, size)
	}
	return tableau.Page{Number: number, Size: size, Total: total}, nil
}
