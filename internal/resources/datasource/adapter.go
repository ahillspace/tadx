// Package datasource isolates published datasource REST behavior.
package datasource

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/value"
)

// Client is the narrow Tableau datasource client used by inventory, identity resolution, and dependency acquisition.
type Client interface {
	Get(context.Context, string) (tableaudatasource.Datasource, error)
	List(context.Context, tableaudatasource.ListRequest) (tableaudatasource.Page, error)
	Download(context.Context, string, *bool) (tableaudatasource.Download, error)
}

// ProjectPathResolver supplies canonical project paths without coupling resource packages.
type ProjectPathResolver interface {
	ResolveProjectPath(context.Context, string) (string, error)
}

// ProjectSelectorPathResolver normalizes a documented display label against the
// full project inventory, so a real project cannot be hidden by an alias.
type ProjectSelectorPathResolver interface {
	ResolveProjectSelectorPath(context.Context, string) (string, error)
}

// Datasource is one normalized authoritative published datasource identity.
type Datasource = value.Datasource

// Page is one bounded normalized published datasource page.
type Page struct {
	Number    int
	Size      int
	Total     int
	Items     []Datasource
	RequestID string
}

// Download is one authoritative datasource and its unchanged native package.
type Download struct {
	LUID             string
	Name             string
	ProjectLUID      string
	ProjectPath      string
	Filename         string
	Content          []byte
	TableauRequestID string
}

// Adapter owns published datasource identity and download sequencing.
type Adapter struct {
	client   Client
	projects ProjectPathResolver
}

const (
	datasourceAdapterPageSize = 1000
)

// NewAdapter creates a datasource resource adapter.
func NewAdapter(client Client) *Adapter {
	return &Adapter{client: client}
}

// NewAdapterWithProjectResolver creates a datasource adapter that supports exact selection.
func NewAdapterWithProjectResolver(client Client, projects ProjectPathResolver) *Adapter {
	return &Adapter{client: client, projects: projects}
}

// ListDatasources returns exactly one validated upstream page.
func (a *Adapter) ListDatasources(ctx context.Context, input tableaudatasource.ListRequest) (Page, error) {
	if a == nil || a.client == nil {
		return Page{}, errors.New("datasource resource adapter is not configured")
	}
	if input.ProjectLUID != "" {
		return a.listDatasourcesByProjectLUID(ctx, input)
	}
	return a.listDatasourcesPage(ctx, input)
}

func (a *Adapter) listDatasourcesPage(ctx context.Context, input tableaudatasource.ListRequest) (Page, error) {
	upstream, err := a.client.List(ctx, input)
	if err != nil {
		return Page{}, err
	}
	if err := validateDatasourcePage(upstream, input.PageNumber, input.PageSize); err != nil {
		return Page{}, err
	}
	items := make([]Datasource, len(upstream.Items))
	seen := make(map[string]tableaudatasource.Datasource, len(upstream.Items))
	for index, item := range upstream.Items {
		if err := recordDatasource(seen, item); err != nil {
			return Page{}, err
		}
		items[index] = normalizeDatasource(item, "")
	}
	return Page{Number: upstream.Number, Size: upstream.Size, Total: upstream.Total, Items: items, RequestID: upstream.TableauRequestID}, nil
}

func (a *Adapter) listDatasourcesByProjectLUID(ctx context.Context, input tableaudatasource.ListRequest) (Page, error) {
	if input.PageNumber <= 0 {
		return Page{}, errors.New("datasource page number must be positive")
	}
	if input.PageSize <= 0 || input.PageSize > datasourceAdapterPageSize {
		return Page{}, fmt.Errorf("datasource page size must be between 1 and %d", datasourceAdapterPageSize)
	}
	request := input
	request.PageSize = datasourceAdapterPageSize
	seen := make(map[string]tableaudatasource.Datasource)
	matches := make([]Datasource, 0)
	expectedTotal, expectedSize := -1, -1
	requestID := ""
	for number := 1; number <= 1000; number++ {
		request.PageNumber = number
		page, err := a.client.List(ctx, request)
		if err != nil {
			return Page{}, err
		}
		if err := validateDatasourcePage(page, number, datasourceAdapterPageSize); err != nil {
			return Page{}, err
		}
		if expectedTotal < 0 {
			expectedTotal, expectedSize = page.Total, page.Size
		} else if page.Total != expectedTotal {
			return Page{}, fmt.Errorf("datasource project filter pagination total changed from %d to %d", expectedTotal, page.Total)
		} else if page.Size != expectedSize {
			return Page{}, fmt.Errorf("datasource project filter pagination size changed from %d to %d", expectedSize, page.Size)
		}
		requestID = page.TableauRequestID
		for _, item := range page.Items {
			_, alreadySeen := seen[item.LUID]
			if err := recordDatasource(seen, item); err != nil {
				return Page{}, err
			}
			if alreadySeen {
				continue
			}
			if item.ProjectLUID == input.ProjectLUID &&
				(input.Name == "" || item.Name == input.Name) &&
				(input.ProjectName == "" || item.ProjectName == input.ProjectName) {
				matches = append(matches, normalizeDatasource(item, ""))
			}
		}
		offset := (page.Number-1)*page.Size + len(page.Items)
		if offset == page.Total {
			break
		}
		if len(page.Items) == 0 {
			return Page{}, errors.New("datasource project filter pagination ended before the reported total")
		}
		if number == 1000 {
			return Page{}, errors.New("datasource project filter exceeded the bounded page limit")
		}
	}
	if expectedTotal < 0 {
		return Page{}, errors.New("datasource project filter returned no page")
	}
	start := (input.PageNumber - 1) * input.PageSize
	if start > len(matches) {
		return Page{}, fmt.Errorf("datasource project filter page %d exceeds the filtered total %d", input.PageNumber, len(matches))
	}
	end := min(start+input.PageSize, len(matches))
	return Page{Number: input.PageNumber, Size: input.PageSize, Total: len(matches), Items: matches[start:end], RequestID: requestID}, nil
}

// ResolveContentURLs maps exact datasource content URLs to authoritative
// records, keyed by content URL. Every URL must resolve to one datasource.
// Native search and workbook files both identify published datasources by
// content URL; only the REST LUID is authoritative for lifecycle operations.
func (a *Adapter) ResolveContentURLs(ctx context.Context, contentURLs []string) (map[string]Datasource, error) {
	if len(contentURLs) == 0 || len(contentURLs) > datasourceAdapterPageSize {
		return nil, fmt.Errorf("datasource content URL resolution requires between 1 and %d values", datasourceAdapterPageSize)
	}
	found, err := a.FindDatasourcesByContentURL(ctx, contentURLs)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]Datasource, len(found))
	for _, item := range found {
		if !slices.Contains(contentURLs, item.ContentURL) {
			return nil, errors.New("datasource content URL resolution returned an unexpected identity")
		}
		if _, duplicate := resolved[item.ContentURL]; duplicate {
			return nil, fmt.Errorf("datasource content URL %q resolved ambiguously", item.ContentURL)
		}
		resolved[item.ContentURL] = item
	}
	for _, contentURL := range contentURLs {
		if _, ok := resolved[contentURL]; !ok {
			return nil, fmt.Errorf("datasource content URL %q did not resolve to a datasource on this site", contentURL)
		}
	}
	return resolved, nil
}

// FindDatasourcesByContentURL returns the datasources whose content URL is
// one of contentURLs. Content URLs with no datasource are absent from the
// result rather than an error.
func (a *Adapter) FindDatasourcesByContentURL(ctx context.Context, contentURLs []string) ([]Datasource, error) {
	return a.findExact(ctx, contentURLs, func(batch []string) tableaudatasource.ListRequest {
		return tableaudatasource.ListRequest{ContentURLs: batch}
	})
}

// FindDatasourcesByName returns the datasources whose name is one of names,
// in any project.
func (a *Adapter) FindDatasourcesByName(ctx context.Context, names []string) ([]Datasource, error) {
	return a.findExact(ctx, names, func(batch []string) tableaudatasource.ListRequest {
		return tableaudatasource.ListRequest{Names: batch}
	})
}

// findExact sends one exact in-filter request per batch of distinct values
// and fails rather than page when a batch matches more than one full page.
func (a *Adapter) findExact(ctx context.Context, values []string, filter func([]string) tableaudatasource.ListRequest) ([]Datasource, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("datasource resource adapter is not configured")
	}
	values = slices.Compact(slices.Sorted(slices.Values(values)))
	seen := map[string]tableaudatasource.Datasource{}
	result := []Datasource{}
	for batch := range slices.Chunk(values, tableaudatasource.MaxFilterValues) {
		request := filter(batch)
		request.PageNumber, request.PageSize = 1, datasourceAdapterPageSize
		page, err := a.client.List(ctx, request)
		if err != nil {
			return nil, err
		}
		if err := validateDatasourcePage(page, 1, datasourceAdapterPageSize); err != nil {
			return nil, err
		}
		if page.Total != len(page.Items) {
			return nil, fmt.Errorf("datasource lookup matched %d datasources, more than one %d-row page", page.Total, page.Size)
		}
		for _, item := range page.Items {
			_, duplicate := seen[item.LUID]
			if err := recordDatasource(seen, item); err != nil {
				return nil, err
			}
			if !duplicate {
				result = append(result, normalizeDatasource(item, ""))
			}
		}
	}
	return result, nil
}

// ResolveDatasource resolves one authoritative LUID or exact name and canonical project path.
func (a *Adapter) ResolveDatasource(ctx context.Context, selector identity.Selector) (Datasource, error) {
	if a == nil || a.client == nil || a.projects == nil {
		return Datasource{}, errors.New("datasource resource adapter is not configured for identity resolution")
	}
	if selector.LUID != "" {
		item, err := a.client.Get(ctx, string(selector.LUID))
		if err != nil {
			return Datasource{}, err
		}
		if strings.TrimSpace(item.LUID) != string(selector.LUID) {
			return Datasource{}, fmt.Errorf("datasource response returned LUID %q, expected %q", item.LUID, selector.LUID)
		}
		if err := validateDatasourceIdentity(item); err != nil {
			return Datasource{}, err
		}
		path, err := a.projects.ResolveProjectPath(ctx, item.ProjectLUID)
		if err != nil {
			return Datasource{}, err
		}
		return normalizeDatasource(item, path), nil
	}
	if strings.TrimSpace(selector.Name) == "" || (strings.TrimSpace(selector.ProjectPath) == "" && strings.TrimSpace(string(selector.ProjectLUID)) == "") {
		return Datasource{}, errors.New("datasource selection requires a LUID or exact name and project path or project LUID")
	}
	if selector.ProjectLUID == "" && selector.ProjectPath != "" {
		if resolver, ok := a.projects.(ProjectSelectorPathResolver); ok {
			path, err := resolver.ResolveProjectSelectorPath(ctx, selector.ProjectPath)
			if err != nil {
				return Datasource{}, err
			}
			selector.ProjectPath = path
		}
	}

	byLUID := make(map[string]Datasource)
	seen := make(map[string]tableaudatasource.Datasource)
	const pageSize = 1000
	expectedTotal, expectedSize := -1, -1
	for number := 1; number <= 1000; number++ {
		page, err := a.client.List(ctx, tableaudatasource.ListRequest{PageNumber: number, PageSize: pageSize, Name: selector.Name, ProjectLUID: string(selector.ProjectLUID)})
		if err != nil {
			return Datasource{}, err
		}
		if err := validateDatasourcePage(page, number, pageSize); err != nil {
			return Datasource{}, err
		}
		if expectedTotal < 0 {
			expectedTotal, expectedSize = page.Total, page.Size
		} else if page.Total != expectedTotal {
			return Datasource{}, fmt.Errorf("datasource pagination total changed from %d to %d", expectedTotal, page.Total)
		} else if page.Size != expectedSize {
			return Datasource{}, fmt.Errorf("datasource pagination size changed from %d to %d", expectedSize, page.Size)
		}
		for _, item := range page.Items {
			if err := recordDatasource(seen, item); err != nil {
				return Datasource{}, err
			}
			if item.Name != selector.Name {
				continue
			}
			if selector.ProjectLUID != "" && item.ProjectLUID != string(selector.ProjectLUID) {
				continue
			}
			path, err := a.projects.ResolveProjectPath(ctx, item.ProjectLUID)
			if err != nil {
				return Datasource{}, err
			}
			if selector.ProjectLUID != "" || path == selector.ProjectPath {
				normalized := normalizeDatasource(item, path)
				normalized.RequestID = page.TableauRequestID
				byLUID[item.LUID] = normalized
			}
		}
		offset := (page.Number-1)*page.Size + len(page.Items)
		if offset == page.Total {
			items := make([]Datasource, 0, len(byLUID))
			for _, item := range byLUID {
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool { return items[i].LUID < items[j].LUID })
			candidates := make([]identity.Candidate, len(items))
			for index, item := range items {
				candidates[index] = identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name, ProjectPath: item.ProjectPath, ProjectLUID: identity.LUID(item.ProjectLUID)}
			}
			resolved, err := identity.Resolve(selector, candidates)
			if err != nil {
				return Datasource{}, err
			}
			selected := byLUID[string(resolved.LUID)]
			selected.RequestID = page.TableauRequestID
			return selected, nil
		}
		if len(page.Items) == 0 {
			return Datasource{}, errors.New("datasource pagination ended before the reported total")
		}
	}
	return Datasource{}, errors.New("datasource resolution exceeded the 1000-page bound")
}

// FindDatasources returns all exact name collisions in one authoritative project.
func (a *Adapter) FindDatasources(ctx context.Context, name, projectLUID string) ([]Datasource, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("datasource resource adapter is not configured")
	}
	name, projectLUID = strings.TrimSpace(name), strings.TrimSpace(projectLUID)
	if name == "" || projectLUID == "" {
		return nil, errors.New("datasource collision lookup requires name and project LUID")
	}
	const pageSize = 1000
	seen := map[string]tableaudatasource.Datasource{}
	result := []Datasource{}
	expectedTotal, expectedSize := -1, -1
	filterName := name
	if strings.Contains(filterName, ",") {
		filterName = ""
	}
	for number := 1; number <= 1000; number++ {
		page, err := a.client.List(ctx, tableaudatasource.ListRequest{PageNumber: number, PageSize: pageSize, Name: filterName})
		if err != nil {
			return nil, err
		}
		if err := validateDatasourcePage(page, number, pageSize); err != nil {
			return nil, err
		}
		if expectedTotal < 0 {
			expectedTotal, expectedSize = page.Total, page.Size
		} else if page.Total != expectedTotal {
			return nil, fmt.Errorf("datasource collision pagination total changed from %d to %d", expectedTotal, page.Total)
		} else if page.Size != expectedSize {
			return nil, fmt.Errorf("datasource collision pagination size changed from %d to %d", expectedSize, page.Size)
		}
		for _, item := range page.Items {
			if err := recordDatasource(seen, item); err != nil {
				return nil, err
			}
			if strings.EqualFold(item.Name, name) && item.ProjectLUID == projectLUID {
				result = append(result, normalizeDatasource(item, ""))
			}
		}
		if (page.Number-1)*page.Size+len(page.Items) == page.Total {
			sort.Slice(result, func(i, j int) bool { return result[i].LUID < result[j].LUID })
			return result, nil
		}
		if len(page.Items) == 0 {
			return nil, errors.New("datasource pagination ended before the reported total")
		}
	}
	return nil, errors.New("datasource collision lookup exceeded the 1000-page bound")
}

func validateDatasourceIdentity(item tableaudatasource.Datasource) error {
	if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.ProjectLUID) == "" || strings.TrimSpace(item.ProjectName) == "" {
		return errors.New("datasource response omitted an authoritative LUID, name, project LUID, or project name")
	}
	return nil
}

func recordDatasource(seen map[string]tableaudatasource.Datasource, item tableaudatasource.Datasource) error {
	if err := validateDatasourceIdentity(item); err != nil {
		return err
	}
	if current, exists := seen[item.LUID]; exists && !reflect.DeepEqual(current, item) {
		return fmt.Errorf("Tableau datasource list returned conflicting records for LUID %q", item.LUID)
	}
	seen[item.LUID] = item
	return nil
}

func validateDatasourcePage(page tableaudatasource.Page, number, requestedSize int) error {
	if number <= 0 || requestedSize <= 0 || page.Number != number || page.Size <= 0 || page.Size > requestedSize || page.Total < 0 || len(page.Items) > page.Size {
		return errors.New("datasource list returned inconsistent pagination")
	}
	offset := int64(page.Number-1) * int64(page.Size)
	if offset > int64(page.Total) || offset+int64(len(page.Items)) > int64(page.Total) {
		return errors.New("datasource list returned inconsistent pagination")
	}
	return nil
}

func normalizeDatasource(item tableaudatasource.Datasource, path string) Datasource {
	return Datasource{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: path,
		Description: item.Description, Type: item.Type, ContentURL: item.ContentURL, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement, RequestID: item.TableauRequestID,
	}
}

// DownloadDatasource reads authoritative identity before downloading native content.
func (a *Adapter) DownloadDatasource(ctx context.Context, luid string) (Download, error) {
	if a == nil || a.client == nil {
		return Download{}, errors.New("datasource adapter is not configured")
	}
	luid = strings.TrimSpace(luid)
	if luid == "" {
		return Download{}, errors.New("datasource LUID is required")
	}
	item, err := a.client.Get(ctx, luid)
	if err != nil {
		return Download{}, err
	}
	if strings.TrimSpace(item.LUID) != luid {
		return Download{}, fmt.Errorf("datasource read returned LUID %q, expected %q", item.LUID, luid)
	}
	if strings.TrimSpace(item.Name) == "" {
		return Download{}, fmt.Errorf("datasource %q omitted its authoritative name", luid)
	}
	if strings.TrimSpace(item.ProjectLUID) == "" {
		return Download{}, fmt.Errorf("datasource %q omitted its authoritative project LUID", luid)
	}
	content, err := a.client.Download(ctx, luid, nil)
	if err != nil {
		return Download{}, err
	}
	return Download{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectName,
		Filename: content.Filename, Content: content.Content, TableauRequestID: content.TableauRequestID,
	}, nil
}
