// Package search implements Tableau's native content exploration search boundary.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	searchPath       = "/api/-/search"
	searchMediaType  = "application/vnd.tableau.search-results.v2+json"
	maxPageSize      = 100
	maxSearchResults = 2000
	maxResponseBytes = 8 * 1024 * 1024
)

// Request describes one bounded native content search page.
type Request struct {
	Terms string
	Types []string
	Limit int
	Page  int
}

// Item is the stable subset shared by supported Tableau content results.
type Item struct {
	LUID        string
	Type        string
	Name        string
	ContentURL  string
	ProjectLUID string
	ProjectName string
	ProjectPath string
	OwnerLUID   string
	OwnerName   string
	ModifiedAt  string
	URI         string
}

// Page is one server-ranked native search page.
type Page struct {
	Items            []Item
	PageIndex        int
	StartIndex       int
	Limit            int
	Total            int
	HasNext          bool
	TableauRequestID string
}

// Client is an authenticated Tableau native search client.
type Client struct {
	transport *tableau.Transport
	session   auth.Session
	serverURL string
}

// AvailabilityError identifies Tableau versions or deployments without the requested search feature.
type AvailabilityError struct {
	reason string
	cause  error
}

func (e *AvailabilityError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("Tableau native content search is unavailable: %s: %v", e.reason, e.cause)
	}
	return "Tableau native content search is unavailable: " + e.reason
}

func (e *AvailabilityError) Unwrap() error { return e.cause }

// NativeSearchUnavailable lets callers distinguish unsupported deployments from empty results.
func (e *AvailabilityError) NativeSearchUnavailable() bool { return true }

// Retryable reports false because retrying cannot add a missing endpoint or feature version.
func (e *AvailabilityError) Retryable() bool { return false }

// CorrectiveAction describes the stable deployment requirement.
func (e *AvailabilityError) CorrectiveAction() string {
	return "Use Tableau Server 2022.3 or later with native content search available."
}

// NewClient creates a native content search client with authenticated site identity.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) (*Client, error) {
	client := &Client{transport: transport, session: session, serverURL: serverURL}
	if client.transport == nil || client.session == nil || strings.TrimSpace(client.serverURL) == "" {
		return nil, errors.New("authenticated Tableau search client is not configured")
	}
	if strings.TrimSpace(client.session.SiteLUID()) == "" {
		return nil, errors.New("authenticated Tableau search client requires a site LUID")
	}
	return client, nil
}

// Search returns one server-ranked page. It never follows the response's next URL.
func (c *Client) Search(ctx context.Context, input Request) (Page, error) {
	request, err := c.normalize(input)
	if err != nil {
		return Page{}, err
	}
	query := url.Values{}
	if request.Terms != "" {
		query.Set("terms", request.Terms)
	}
	if len(request.Types) == 1 {
		query.Set("filter", "type:eq:"+request.Types[0])
	} else {
		query.Set("filter", "type:in:["+strings.Join(request.Types, ",")+"]")
	}
	query.Set("limit", strconv.Itoa(request.Limit))
	query.Set("page", strconv.Itoa(request.Page))
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodGet, ServerURL: c.serverURL, Path: searchPath, Query: query,
		Header: http.Header{"X-Tableau-Site-Id": []string{c.session.SiteLUID()}},
		Accept: searchMediaType, Operation: "content.search", MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		if httpStatus(err) == http.StatusNotFound || httpStatus(err) == http.StatusMethodNotAllowed {
			return Page{}, &AvailabilityError{reason: "the independently versioned /api/-/search resource is not available", cause: err}
		}
		return Page{}, err
	}
	return decodePage(response, request)
}

func (c *Client) normalize(input Request) (Request, error) {
	major, minor, err := apiVersion(c.transport.APIVersion())
	if err != nil {
		return Request{}, &AvailabilityError{reason: "the configured Tableau API version is invalid", cause: err}
	}
	if major < 3 || (major == 3 && minor < 16) {
		return Request{}, &AvailabilityError{reason: "Tableau Server 2022.3 or Tableau REST API 3.16 and later is required"}
	}
	input.Terms = strings.TrimSpace(input.Terms)
	if input.Limit < 1 || input.Limit > maxPageSize {
		return Request{}, fmt.Errorf("native search limit must be between 1 and %d", maxPageSize)
	}
	if input.Page < 0 || input.Page > (maxSearchResults-1)/input.Limit {
		return Request{}, fmt.Errorf("native search page exceeds the bounded %d-result window", maxSearchResults)
	}
	seen := make(map[string]bool, len(input.Types))
	types := make([]string, 0, len(input.Types))
	for _, candidate := range input.Types {
		kind := strings.ToLower(strings.TrimSpace(candidate))
		if !supportedType(kind) {
			return Request{}, fmt.Errorf("native search does not support TADX type %q", candidate)
		}
		if !seen[kind] {
			seen[kind] = true
			types = append(types, kind)
		}
	}
	if len(types) == 0 {
		return Request{}, errors.New("native search requires at least one supported content type")
	}
	sort.Strings(types)
	if len(types) > 1 && major == 3 && minor < 17 {
		return Request{}, &AvailabilityError{reason: "multi-type native search requires Tableau REST API 3.17 and later"}
	}
	input.Types = types
	return input, nil
}

func decodePage(response tableau.Response, request Request) (Page, error) {
	type pageEnvelope struct {
		Items      json.RawMessage `json:"items"`
		Limit      *int            `json:"limit"`
		PageIndex  *int            `json:"pageIndex"`
		StartIndex *int            `json:"startIndex"`
		Total      *int            `json:"total"`
		Next       string          `json:"next"`
	}
	var top struct {
		pageEnvelope
		Hits json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(response.Body, &top); err != nil {
		return Page{}, tableau.NewProtocolError("content.search", response, err, true)
	}
	envelope := top.pageEnvelope
	if top.Hits != nil {
		if top.Items != nil || top.Limit != nil || top.PageIndex != nil || top.StartIndex != nil || top.Total != nil {
			return Page{}, tableau.NewProtocolError("content.search", response, errors.New("search response returned ambiguous pagination envelopes"), true)
		}
		if err := json.Unmarshal(top.Hits, &envelope); err != nil {
			return Page{}, tableau.NewProtocolError("content.search", response, err, true)
		}
	}
	if envelope.Limit == nil || envelope.PageIndex == nil || envelope.StartIndex == nil || envelope.Total == nil {
		err := errors.New("search response omitted required pagination fields")
		return Page{}, tableau.NewProtocolError("content.search", response, err, true)
	}
	if envelope.Items == nil && *envelope.Total == 0 {
		envelope.Items = json.RawMessage("[]")
	}
	if envelope.Items == nil {
		err := errors.New("search response omitted items for nonempty pagination")
		return Page{}, tableau.NewProtocolError("content.search", response, err, true)
	}
	var rawItems []rawItem
	if err := json.Unmarshal(envelope.Items, &rawItems); err != nil || rawItems == nil {
		if err == nil {
			err = errors.New("search response requires an items array")
		}
		return Page{}, tableau.NewProtocolError("content.search", response, err, true)
	}
	if *envelope.Limit != request.Limit || *envelope.PageIndex != request.Page || *envelope.StartIndex != request.Page*request.Limit || *envelope.Total < 0 || *envelope.StartIndex+len(rawItems) > *envelope.Total || len(rawItems) > request.Limit {
		return Page{}, tableau.NewProtocolError("content.search", response, errors.New("search response returned inconsistent pagination"), true)
	}
	requested := make(map[string]bool, len(request.Types))
	for _, kind := range request.Types {
		requested[kind] = true
	}
	items := make([]Item, len(rawItems))
	seen := make(map[string]bool, len(rawItems))
	for i, raw := range rawItems {
		item := raw.normalize()
		key := item.Type + "\x00" + item.LUID
		if !requested[item.Type] || item.LUID == "" || item.Name == "" || seen[key] {
			return Page{}, tableau.NewProtocolError("content.search", response, errors.New("search response returned invalid or duplicate authoritative content identity"), true)
		}
		seen[key] = true
		items[i] = item
	}
	nextStart := *envelope.StartIndex + len(items)
	hasMore := nextStart < *envelope.Total && nextStart < maxSearchResults
	if hasMore && strings.TrimSpace(envelope.Next) == "" {
		return Page{}, tableau.NewProtocolError("content.search", response, errors.New("search response omitted continuation for remaining results"), true)
	}
	return Page{Items: items, PageIndex: *envelope.PageIndex, StartIndex: *envelope.StartIndex, Limit: *envelope.Limit, Total: *envelope.Total, HasNext: hasMore, TableauRequestID: response.TableauRequestID}, nil
}

type rawItem struct {
	URI     string     `json:"uri"`
	Content rawContent `json:"content"`
}

type rawContent struct {
	LUID          string      `json:"luid"`
	Type          string      `json:"type"`
	ContentType   string      `json:"contentType"`
	Name          string      `json:"name"`
	Title         string      `json:"title"`
	ProjectLUID   string      `json:"projectLuid"`
	ProjectName   string      `json:"projectName"`
	ProjectPath   string      `json:"projectPath"`
	OwnerLUID     string      `json:"ownerLuid"`
	OwnerName     string      `json:"ownerName"`
	ContainerName string      `json:"containerName"`
	ContainerType string      `json:"containerType"`
	RepositoryURL string      `json:"repositoryUrl"`
	ModifiedTime  string      `json:"modifiedTime"`
	UpdatedAt     string      `json:"updatedAt"`
	Project       rawIdentity `json:"project"`
	Owner         rawIdentity `json:"owner"`
}

type rawIdentity struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
}

func (item rawItem) normalize() Item {
	content := item.Content
	kind := normalizeContentType(content.Type)
	if kind == "" {
		kind = normalizeContentType(content.ContentType)
	}
	luid := content.LUID
	name := content.Name
	if name == "" {
		name = content.Title
	}
	projectLUID := content.ProjectLUID
	if projectLUID == "" {
		projectLUID = content.Project.LUID
	}
	projectName := content.ProjectName
	if projectName == "" {
		projectName = content.Project.Name
	}
	if projectName == "" && strings.EqualFold(strings.TrimSpace(content.ContainerType), "project") {
		projectName = content.ContainerName
	}
	ownerLUID := content.OwnerLUID
	if ownerLUID == "" {
		ownerLUID = content.Owner.LUID
	}
	ownerName := content.OwnerName
	if ownerName == "" {
		ownerName = content.Owner.Name
	}
	modifiedAt := content.ModifiedTime
	if modifiedAt == "" {
		modifiedAt = content.UpdatedAt
	}
	return Item{
		LUID: strings.TrimSpace(luid), Type: strings.ToLower(strings.TrimSpace(kind)), Name: strings.TrimSpace(name),
		ContentURL:  strings.TrimSpace(content.RepositoryURL),
		ProjectLUID: strings.TrimSpace(projectLUID), ProjectName: strings.TrimSpace(projectName), ProjectPath: strings.TrimSpace(content.ProjectPath),
		OwnerLUID: strings.TrimSpace(ownerLUID), OwnerName: strings.TrimSpace(ownerName), ModifiedAt: strings.TrimSpace(modifiedAt), URI: strings.TrimSpace(item.URI),
	}
}

func normalizeContentType(value string) string {
	kind := strings.ToLower(strings.TrimSpace(value))
	switch kind {
	case "unifieddatasource":
		return "datasource"
	default:
		return kind
	}
}

func supportedType(kind string) bool {
	switch kind {
	case "workbook", "datasource", "flow", "project":
		return true
	default:
		return false
	}
}

func apiVersion(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid API version %q", value)
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 0 || minor < 0 {
		return 0, 0, fmt.Errorf("invalid API version %q", value)
	}
	return major, minor, nil
}

func httpStatus(err error) int {
	var carrier interface{ HTTPStatus() int }
	if errors.As(err, &carrier) {
		return carrier.HTTPStatus()
	}
	return 0
}
