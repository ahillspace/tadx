// Package datasource implements the released Tableau datasource REST client family.
package datasource

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	maxPageSize            = 1000
	maxListResponseBytes   = 16 * 1024 * 1024
	defaultUploadThreshold = 64 * 1024 * 1024
	defaultUploadChunkSize = 64 * 1024 * 1024
)

// Datasource is the bounded REST metadata used by inventory and workbook dependency acquisition.
type Datasource struct {
	LUID                string
	Name                string
	ProjectLUID         string
	ProjectName         string
	Description         string
	Type                string
	ContentURL          string
	OwnerLUID           string
	CreatedAt           string
	UpdatedAt           string
	Size                *int64
	EncryptExtracts     *bool
	HasExtracts         *bool
	IsCertified         *bool
	CertificationNote   string
	UseRemoteQueryAgent *bool
	WebpageURL          string
	Tags                []string
	AskDataEnablement   string
	TableauRequestID    string
}

// ListRequest selects one bounded published datasource page.
type ListRequest struct {
	PageNumber    int
	PageSize      int
	Name          string
	OwnerName     string
	ProjectLUID   string
	ProjectName   string
	Type          string
	Tag           string
	UpdatedAfter  string
	UpdatedBefore string
	ContentURLs   []string
}

const maxContentURLResolveBatch = 25

// ResolveContentURLs maps native-search datasource content URLs to classic
// REST LUIDs in bounded batches. Search-service datasource LUIDs are not
// authoritative for classic lifecycle operations.
func (c *Client) ResolveContentURLs(ctx context.Context, contentURLs []string) (map[string]string, error) {
	if len(contentURLs) == 0 || len(contentURLs) > maxPageSize {
		return nil, fmt.Errorf("datasource content URL resolution requires between 1 and %d values", maxPageSize)
	}
	unique := make([]string, 0, len(contentURLs))
	requested := make(map[string]struct{}, len(contentURLs))
	for index, value := range contentURLs {
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsAny(value, ",&") {
			return nil, fmt.Errorf("datasource content URL %d is empty or cannot be represented in a Tableau filter", index)
		}
		if _, exists := requested[value]; exists {
			continue
		}
		requested[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	resolved := make(map[string]string, len(unique))
	for start := 0; start < len(unique); start += maxContentURLResolveBatch {
		end := start + maxContentURLResolveBatch
		if end > len(unique) {
			end = len(unique)
		}
		batch := unique[start:end]
		page, err := c.List(ctx, ListRequest{PageNumber: 1, PageSize: len(batch), ContentURLs: batch})
		if err != nil {
			return nil, err
		}
		if page.Total != len(page.Items) {
			return nil, errors.New("datasource content URL resolution exceeded its bounded result page")
		}
		batchRequested := make(map[string]struct{}, len(batch))
		for _, value := range batch {
			batchRequested[value] = struct{}{}
		}
		for _, item := range page.Items {
			if _, expected := batchRequested[item.ContentURL]; !expected || strings.TrimSpace(item.LUID) == "" {
				return nil, errors.New("datasource content URL resolution returned an unexpected identity")
			}
			if _, duplicate := resolved[item.ContentURL]; duplicate {
				return nil, fmt.Errorf("datasource content URL %q resolved ambiguously", item.ContentURL)
			}
			resolved[item.ContentURL] = item.LUID
		}
	}
	for _, value := range unique {
		if resolved[value] == "" {
			return nil, fmt.Errorf("datasource content URL %q did not resolve to a classic REST LUID", value)
		}
	}
	return resolved, nil
}

// Page contains one normalized classic REST datasource page.
type Page struct {
	Number           int
	Size             int
	Total            int
	Items            []Datasource
	TableauRequestID string
}

// Download is a preserved native datasource response.
type Download struct {
	Filename         string
	ContentType      string
	Content          []byte
	TableauRequestID string
}

// Client reads published datasource metadata and native content.
type Client struct {
	transport        *tableau.Transport
	session          auth.Session
	serverURL        string
	maxDownloadBytes int64
	uploadThreshold  int64
	uploadChunkSize  int64
	pollInterval     time.Duration
	pollTimeout      time.Duration
}

// NewClient creates an authenticated datasource REST client.
func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{transport: transport, session: session, serverURL: serverURL, uploadThreshold: defaultUploadThreshold, uploadChunkSize: defaultUploadChunkSize, pollInterval: time.Second, pollTimeout: 10 * time.Minute}
}

func (c *Client) SetUploadThreshold(limit int64) {
	if c != nil && limit > 0 && limit <= defaultUploadThreshold {
		c.uploadThreshold = limit
	}
}
func (c *Client) SetUploadChunkSize(limit int64) {
	if c != nil && limit > 0 && limit <= defaultUploadChunkSize {
		c.uploadChunkSize = limit
	}
}

// SetPollPolicy configures bounded internal asynchronous publish polling.
func (c *Client) SetPollPolicy(interval, timeout time.Duration) {
	if c == nil {
		return
	}
	if interval > 0 {
		c.pollInterval = interval
	}
	if timeout > 0 {
		c.pollTimeout = timeout
	}
}

// SetMaxDownloadBytes bounds the buffered native datasource download below the shared transport ceiling.
// Zero keeps the shared 256 MiB limit.
func (c *Client) SetMaxDownloadBytes(limit int64) {
	if limit > 0 {
		c.maxDownloadBytes = limit
	}
}

// List returns one bounded published datasource page.
func (c *Client) List(ctx context.Context, input ListRequest) (Page, error) {
	if err := c.validate(); err != nil {
		return Page{}, err
	}
	query, err := datasourceListQuery(input)
	if err != nil {
		return Page{}, err
	}
	response, err := c.do(ctx, c.sitePath("datasources"), "datasource.list", maxListResponseBytes, query)
	if err != nil {
		return Page{}, err
	}
	var envelope datasourceListEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Page{}, tableau.NewProtocolError("datasource.list", response, fmt.Errorf("decode datasource list response: %w", err), true)
	}
	if len(envelope.Pagination) != 1 {
		return Page{}, tableau.NewProtocolError("datasource.list", response, fmt.Errorf("datasource list response contained %d pagination elements; expected 1", len(envelope.Pagination)), true)
	}
	if len(envelope.Datasources) != 1 {
		return Page{}, tableau.NewProtocolError("datasource.list", response, fmt.Errorf("datasource list response contained %d datasources elements; expected 1", len(envelope.Datasources)), true)
	}
	itemsXML := envelope.Datasources[0].Items
	page, err := normalizeDatasourcePagination(envelope.Pagination[0], input.PageNumber, input.PageSize, len(itemsXML))
	if err != nil {
		return Page{}, tableau.NewProtocolError("datasource.list", response, err, true)
	}
	items := make([]Datasource, len(itemsXML))
	seen := make(map[string]Datasource, len(itemsXML))
	for index, item := range itemsXML {
		datasource := normalizeDatasource(item)
		if datasource.LUID == "" || datasource.Name == "" || datasource.ProjectLUID == "" || datasource.ProjectName == "" {
			return Page{}, tableau.NewProtocolError("datasource.list", response, fmt.Errorf("datasource list response returned an incomplete authoritative identity at item %d", index), true)
		}
		if current, exists := seen[datasource.LUID]; exists && !reflect.DeepEqual(current, datasource) {
			return Page{}, tableau.NewProtocolError("datasource.list", response, fmt.Errorf("datasource list response returned conflicting records for LUID %q", datasource.LUID), true)
		}
		seen[datasource.LUID] = datasource
		items[index] = datasource
	}
	return Page{Number: page.Number, Size: page.Size, Total: page.Total, Items: items, TableauRequestID: response.TableauRequestID}, nil
}

// Get returns one published datasource by its authoritative LUID.
func (c *Client) Get(ctx context.Context, datasourceLUID string) (Datasource, error) {
	if datasourceLUID == "" {
		return Datasource{}, errors.New("datasource LUID is required")
	}
	if err := c.validate(); err != nil {
		return Datasource{}, err
	}
	response, err := c.do(ctx, c.sitePath("datasources", datasourceLUID), "datasource.get", maxListResponseBytes, nil)
	if err != nil {
		return Datasource{}, err
	}
	var envelope datasourceGetEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, fmt.Errorf("decode datasource response: %w", err), true)
	}
	result := normalizeDatasource(envelope.Datasource)
	result.TableauRequestID = response.TableauRequestID
	if result.LUID != datasourceLUID {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, fmt.Errorf("datasource response returned LUID %q, expected %q", result.LUID, datasourceLUID), true)
	}
	if result.Name == "" {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, errors.New("datasource response omitted name"), true)
	}
	if result.ProjectLUID == "" {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, errors.New("datasource response omitted project LUID"), true)
	}
	if result.ProjectName == "" {
		return Datasource{}, tableau.NewProtocolError("datasource.get", response, errors.New("datasource response omitted project name"), true)
	}
	return result, nil
}

// Download preserves the native TDS or TDSX bytes and filename.
func (c *Client) Download(ctx context.Context, datasourceLUID string, includeExtract *bool) (Download, error) {
	if datasourceLUID == "" {
		return Download{}, errors.New("datasource LUID is required")
	}
	if err := c.validate(); err != nil {
		return Download{}, err
	}
	query := url.Values{}
	if includeExtract != nil {
		query.Set("includeExtract", strconv.FormatBool(*includeExtract))
	}
	response, err := c.do(ctx, c.sitePath("datasources", datasourceLUID, "content"), "datasource.pull", c.maxDownloadBytes, query)
	if err != nil {
		return Download{}, err
	}
	filename := dispositionFilename(response.Header.Get("Content-Disposition"))
	if filename == "" {
		return Download{}, tableau.NewProtocolError("datasource.pull", response, errors.New("datasource download response omitted filename"), true)
	}
	if !validFilename(filename) {
		return Download{}, tableau.NewProtocolError("datasource.pull", response, fmt.Errorf("datasource download returned invalid filename %q", filename), true)
	}
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != ".tds" && extension != ".tdsx" {
		return Download{}, tableau.NewProtocolError("datasource.pull", response, fmt.Errorf("datasource download returned unsupported filename %q", filename), true)
	}
	return Download{
		Filename:         filename,
		ContentType:      response.Header.Get("Content-Type"),
		Content:          response.Body,
		TableauRequestID: response.TableauRequestID,
	}, nil
}

func (c *Client) do(ctx context.Context, path, operation string, maxResponseBytes int64, query url.Values) (tableau.Response, error) {
	if err := c.validate(); err != nil {
		return tableau.Response{}, err
	}
	return c.transport.Do(ctx, c.session, tableau.Request{
		Method:           http.MethodGet,
		ServerURL:        c.serverURL,
		Path:             path,
		Query:            query,
		Accept:           "application/xml",
		Operation:        operation,
		MaxResponseBytes: maxResponseBytes,
	})
}

func (c *Client) validate() error {
	if c == nil || c.transport == nil || c.session == nil {
		return errors.New("authenticated datasource client is not configured")
	}
	return nil
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
	for _, candidate := range []string{value, "attachment; " + value} {
		_, parameters, err := mime.ParseMediaType(candidate)
		if err == nil && parameters["filename"] != "" {
			return parameters["filename"]
		}
	}
	return ""
}

func validFilename(filename string) bool {
	return filename != "." && filename != ".." &&
		!strings.ContainsAny(filename, `/\`) && filepath.Base(filename) == filename
}

type datasourceGetEnvelope struct {
	Datasource datasourceXML `xml:"datasource"`
}

type datasourceListEnvelope struct {
	Pagination  []paginationXML     `xml:"pagination"`
	Datasources []datasourceListXML `xml:"datasources"`
}

type datasourceListXML struct {
	Items []datasourceXML `xml:"datasource"`
}

type datasourceXML struct {
	ID                  string `xml:"id,attr"`
	Name                string `xml:"name,attr"`
	Description         string `xml:"description,attr"`
	Type                string `xml:"type,attr"`
	ContentURL          string `xml:"contentUrl,attr"`
	CreatedAt           string `xml:"createdAt,attr"`
	UpdatedAt           string `xml:"updatedAt,attr"`
	Size                *int64 `xml:"size,attr"`
	EncryptExtracts     *bool  `xml:"encryptExtracts,attr"`
	HasExtracts         *bool  `xml:"hasExtracts,attr"`
	IsCertified         *bool  `xml:"isCertified,attr"`
	CertificationNote   string `xml:"certificationNote,attr"`
	UseRemoteQueryAgent *bool  `xml:"useRemoteQueryAgent,attr"`
	WebpageURL          string `xml:"webpageUrl,attr"`
	Project             struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	} `xml:"project"`
	Owner struct {
		ID string `xml:"id,attr"`
	} `xml:"owner"`
	Tags []struct {
		Label string `xml:"label,attr"`
	} `xml:"tags>tag"`
	AskData struct {
		Enablement string `xml:"enablement,attr"`
	} `xml:"askData"`
}

type paginationXML struct {
	Number *int `xml:"pageNumber,attr"`
	Size   *int `xml:"pageSize,attr"`
	Total  *int `xml:"totalAvailable,attr"`
}

func datasourceListQuery(input ListRequest) (url.Values, error) {
	if input.PageNumber <= 0 {
		return nil, errors.New("datasource page number must be positive")
	}
	if input.PageSize <= 0 || input.PageSize > maxPageSize {
		return nil, fmt.Errorf("datasource page size must be between 1 and %d", maxPageSize)
	}
	query := url.Values{
		"pageNumber": {strconv.Itoa(input.PageNumber)},
		"pageSize":   {strconv.Itoa(input.PageSize)},
		"sort":       {"name:asc,updatedAt:asc"},
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

// ListFilter validates and encodes datasource selectors for paged and full lists.
// Pagination fields do not affect the selected population.
func ListFilter(input ListRequest) (string, error) {
	var after, before time.Time
	for _, bound := range []struct {
		name, value string
		target      *time.Time
	}{
		{"updated-after", input.UpdatedAfter, &after}, {"updated-before", input.UpdatedBefore, &before},
	} {
		if bound.value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, bound.value)
		if err != nil {
			return "", fmt.Errorf("datasource %s filter must be an RFC3339 timestamp", bound.name)
		}
		*bound.target = parsed
	}
	if input.UpdatedAfter != "" && input.UpdatedBefore != "" && after.After(before) {
		return "", errors.New("datasource updated-after filter must not be later than updated-before")
	}
	fields := []struct {
		name     string
		operator string
		value    string
	}{
		{name: "name", operator: "eq", value: input.Name},
		{name: "ownerName", operator: "eq", value: input.OwnerName},
		{name: "projectId", operator: "eq", value: input.ProjectLUID},
		{name: "projectName", operator: "eq", value: input.ProjectName},
		{name: "type", operator: "eq", value: input.Type},
		{name: "tags", operator: "eq", value: input.Tag},
		{name: "updatedAt", operator: "gte", value: input.UpdatedAfter},
		{name: "updatedAt", operator: "lte", value: input.UpdatedBefore},
	}
	filters := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		if strings.ContainsAny(field.value, ",&") {
			return "", fmt.Errorf("datasource filter %s cannot contain ampersand or comma", field.name)
		}
		filters = append(filters, field.name+":"+field.operator+":"+field.value)
	}
	if len(input.ContentURLs) > 0 {
		values := make([]string, len(input.ContentURLs))
		for index, value := range input.ContentURLs {
			value = strings.TrimSpace(value)
			if value == "" || strings.ContainsAny(value, ",&") {
				return "", fmt.Errorf("datasource filter contentUrl value %d cannot be empty or contain ampersand or comma", index)
			}
			values[index] = value
		}
		operator, value := "eq", values[0]
		if len(values) > 1 {
			operator, value = "in", "["+strings.Join(values, ",")+"]"
		}
		filters = append(filters, "contentUrl:"+operator+":"+value)
	}
	return strings.Join(filters, ","), nil
}

func normalizeDatasource(item datasourceXML) Datasource {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		label := strings.TrimSpace(tag.Label)
		if label != "" {
			tags = append(tags, label)
		}
	}
	sort.Strings(tags)
	return Datasource{
		LUID: strings.TrimSpace(item.ID), Name: strings.TrimSpace(item.Name),
		ProjectLUID: strings.TrimSpace(item.Project.ID), ProjectName: strings.TrimSpace(item.Project.Name),
		Description: item.Description, Type: strings.TrimSpace(item.Type), ContentURL: strings.TrimSpace(item.ContentURL),
		OwnerLUID: strings.TrimSpace(item.Owner.ID), CreatedAt: strings.TrimSpace(item.CreatedAt), UpdatedAt: strings.TrimSpace(item.UpdatedAt),
		Size: item.Size, EncryptExtracts: item.EncryptExtracts, HasExtracts: item.HasExtracts, IsCertified: item.IsCertified,
		CertificationNote: item.CertificationNote, UseRemoteQueryAgent: item.UseRemoteQueryAgent,
		WebpageURL: strings.TrimSpace(item.WebpageURL), Tags: tags, AskDataEnablement: strings.TrimSpace(item.AskData.Enablement),
	}
}

func normalizeDatasourcePagination(value paginationXML, requestedNumber, requestedSize, itemCount int) (tableau.Page, error) {
	if value.Number == nil || value.Size == nil || value.Total == nil {
		return tableau.Page{}, errors.New("datasource list response omitted required pagination attributes")
	}
	number, size, total := *value.Number, *value.Size, *value.Total
	if number != requestedNumber || number <= 0 {
		return tableau.Page{}, fmt.Errorf("datasource list response returned page number %d, expected %d", number, requestedNumber)
	}
	if size <= 0 || size > requestedSize || size > maxPageSize {
		return tableau.Page{}, fmt.Errorf("datasource list response returned invalid page size %d", size)
	}
	if total < 0 || itemCount > size || total < itemCount {
		return tableau.Page{}, fmt.Errorf("datasource list response returned inconsistent pagination total %d, size %d, and item count %d", total, size, itemCount)
	}
	pageIndex := int64(number - 1)
	if pageIndex > math.MaxInt64/int64(size) {
		return tableau.Page{}, fmt.Errorf("datasource list response page %d exceeds the pagination bound", number)
	}
	offset := pageIndex * int64(size)
	remaining := int64(total) - offset
	if remaining < 0 {
		return tableau.Page{}, fmt.Errorf("datasource list response total %d is inconsistent with page %d", total, number)
	}
	expected := min(int64(size), remaining)
	if int64(itemCount) != expected {
		return tableau.Page{}, fmt.Errorf("datasource list response returned %d items for page %d; expected %d from total %d and size %d", itemCount, number, expected, total, size)
	}
	return tableau.Page{Number: number, Size: size, Total: total}, nil
}
