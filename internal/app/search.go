package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	adminlist "github.com/ahillspace/tadx/actions/admin/group/list"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	searchaction "github.com/ahillspace/tadx/actions/search"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/readsource"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

// searchCommands composes the live-first global search action.
type searchCommands struct{ runtime *runtimeDependencies }

func newSearchCommands(runtime *runtimeDependencies) *searchCommands {
	return &searchCommands{runtime: runtime}
}

func (c *searchCommands) Execute(ctx context.Context, input searchaction.Input) (searchaction.Output, error) {
	if input.Catalog {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return searchaction.Output{}, capabilitySetupError("search.catalog.setup", "search", input.Environment, input.Site, "Catalog search setup failed.", "Review the selected environment and catalog configuration.", err)
		}
		input.Environment = environment.Alias
		if input.Site == "" {
			input.Site = environment.SiteContentURL
		}
		input.SiteResolved = true
		store := catalog.NewStore(filepath.Dir(c.runtime.configPath), c.runtime.now)
		return searchaction.New(catalogGlobalSearchSource{store: store}).Execute(ctx, input)
	}
	if strings.TrimSpace(input.Terms) == "" && completeListSearchSelector(input.Type) {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return searchaction.Output{}, remoteSetupError("search", input.Environment, input.Site, environment, err)
		}
		input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
		lister := &completeLiveSearchLister{
			environment: environment.Alias,
			content:     newRemoteContentCommands(c.runtime),
			admin:       newRemoteAdminCommands(c.runtime),
		}
		return searchaction.New(globalSearchSource{lists: &completeLiveSearchAdapter{lister: lister}}).Execute(ctx, input)
	}

	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return searchaction.Output{}, remoteSetupError("search", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site, input.SiteResolved = connection.environment.Alias, connection.environment.SiteContentURL, true
	lister, err := newLiveSearchLister(connection)
	if err != nil {
		return searchaction.Output{}, remoteSetupError("search", input.Environment, input.Site, connection.environment, err)
	}
	nativeClient, err := tableausearch.NewClient(connection.transport, connection.session, connection.environment.URL)
	if err != nil {
		return searchaction.Output{}, remoteSetupError("search", input.Environment, input.Site, connection.environment, err)
	}
	datasourceResolver := tableaudatasource.NewClient(connection.transport, connection.session, connection.environment.URL)
	return searchaction.New(globalSearchSource{native: resourcesearch.NewNativeAdapter(nativeClient, datasourceResolver), dedicated: resourcesearch.NewAdapter(lister)}).Execute(ctx, input)
}

type appSearchAdapter interface {
	Search(context.Context, resourcesearch.Input) (resourcesearch.Page, error)
}

type appBoundedSearchAdapter interface {
	SearchBounded(context.Context, resourcesearch.Input, int) (resourcesearch.Page, error)
}

type globalSearchSource struct {
	native    appSearchAdapter
	lists     appSearchAdapter
	dedicated appSearchAdapter
}

func (s globalSearchSource) Search(ctx context.Context, input searchaction.Input) (searchaction.Result, error) {
	types, err := searchaction.Types(input.Type)
	if err != nil {
		return searchaction.Result{}, err
	}
	resourceInput := resourcesearch.Input{Types: types, Terms: input.Terms, ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: input.Cursor, Limit: input.Limit}
	if strings.TrimSpace(input.Terms) == "" {
		if completeListSearchSelector(input.Type) {
			return executeAppSearch(ctx, s.lists, resourceInput)
		}
		return executeAppSearch(ctx, s.dedicated, resourceInput)
	}
	if dedicatedSearchSelector(input.Type) {
		return executeAppSearch(ctx, s.dedicated, resourceInput)
	}
	if contentSearchSelector(input.Type) {
		return executeAppSearch(ctx, s.native, resourceInput)
	}
	if input.Type != "" {
		return searchaction.Result{}, fmt.Errorf("unsupported live search selector %q", input.Type)
	}
	return s.searchAll(ctx, input)
}

func completeListSearchSelector(selector string) bool {
	switch selector {
	case "content", "admin", "workbook", "datasource", "flow", "project", "user", "group":
		return true
	default:
		return false
	}
}

func executeAppSearch(ctx context.Context, adapter appSearchAdapter, input resourcesearch.Input) (searchaction.Result, error) {
	if adapter == nil {
		return searchaction.Result{}, errors.New("live search adapter is not configured")
	}
	page, err := adapter.Search(ctx, input)
	if err != nil {
		return searchaction.Result{}, err
	}
	return searchResult(page, nil), nil
}

func dedicatedSearchSelector(selector string) bool {
	switch selector {
	case "admin", "user", "group", "pulse", "definition", "metric":
		return true
	default:
		return false
	}
}

func contentSearchSelector(selector string) bool {
	switch selector {
	case "content", "workbook", "datasource", "flow", "project":
		return true
	default:
		return false
	}
}

const (
	combinedSearchContent   = "content"
	combinedSearchDedicated = "dedicated"
	maxCombinedCursorBytes  = 8192
	maxCombinedSourceBytes  = 4096
)

type combinedSearchCursor struct {
	Version     int    `json:"v"`
	Fingerprint string `json:"f"`
	Phase       string `json:"p"`
	Source      string `json:"c,omitempty"`
	Checksum    string `json:"s"`
}

type invalidCombinedSearchCursor struct{}

func (invalidCombinedSearchCursor) Error() string             { return "combined live search cursor is invalid" }
func (invalidCombinedSearchCursor) InvalidSearchCursor() bool { return true }

func (s globalSearchSource) searchAll(ctx context.Context, input searchaction.Input) (searchaction.Result, error) {
	if s.native == nil || s.dedicated == nil {
		return searchaction.Result{}, errors.New("live search adapters are not configured")
	}
	state, err := decodeCombinedSearchCursor(input.Cursor, input)
	if err != nil {
		return searchaction.Result{}, err
	}
	if state.Phase == combinedSearchContent {
		page, err := s.native.Search(ctx, resourcesearch.Input{
			Types: []string{"datasource", "flow", "project", "workbook"}, Terms: input.Terms,
			ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: state.Source, Limit: input.Limit,
		})
		if err != nil {
			return searchaction.Result{}, err
		}
		if page.NextCursor != "" {
			if len(page.NextCursor) > maxCombinedSourceBytes {
				return searchaction.Result{}, errors.New("native search continuation exceeded the combined cursor bound")
			}
			page.Total = 0
			page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchContent, Source: page.NextCursor})
			return searchResult(page, nil), nil
		}
		if len(page.Items) == input.Limit {
			page.Total = 0
			page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchDedicated})
			return searchResult(page, nil), nil
		}
		state.Phase = combinedSearchDedicated
		state.Source = ""
		remaining := input.Limit - len(page.Items)
		dedicatedPage, err := executeBoundedAppSearch(ctx, s.dedicated, resourcesearch.Input{
			Types: []string{"definition", "group", "metric", "user"}, Terms: input.Terms,
			ProjectPath: input.ProjectPath, Owner: input.Owner, Limit: input.Limit,
		}, remaining)
		if err != nil {
			return searchaction.Result{}, err
		}
		page.Items = append(page.Items, dedicatedPage.Items...)
		page.Total = 0
		page.Warnings = append(page.Warnings, dedicatedPage.Warnings...)
		if dedicatedPage.TableauRequestID != "" {
			page.TableauRequestID = dedicatedPage.TableauRequestID
		}
		if dedicatedPage.NextCursor != "" {
			if len(dedicatedPage.NextCursor) > maxCombinedSourceBytes {
				return searchaction.Result{}, errors.New("dedicated search continuation exceeded the combined cursor bound")
			}
			page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchDedicated, Source: dedicatedPage.NextCursor})
		}
		return searchResult(page, nil), nil
	}
	page, err := s.dedicated.Search(ctx, resourcesearch.Input{
		Types: []string{"definition", "group", "metric", "user"}, Terms: input.Terms,
		ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: state.Source, Limit: input.Limit,
	})
	if err != nil {
		return searchaction.Result{}, err
	}
	page.Total = 0
	if page.NextCursor != "" {
		if len(page.NextCursor) > maxCombinedSourceBytes {
			return searchaction.Result{}, errors.New("dedicated search continuation exceeded the combined cursor bound")
		}
		page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchDedicated, Source: page.NextCursor})
	}
	return searchResult(page, nil), nil
}

func executeBoundedAppSearch(ctx context.Context, adapter appSearchAdapter, input resourcesearch.Input, budget int) (resourcesearch.Page, error) {
	if budget == input.Limit {
		return adapter.Search(ctx, input)
	}
	bounded, ok := adapter.(appBoundedSearchAdapter)
	if !ok {
		return resourcesearch.Page{}, errors.New("dedicated search adapter does not support a partial row budget")
	}
	return bounded.SearchBounded(ctx, input, budget)
}

func decodeCombinedSearchCursor(value string, input searchaction.Input) (combinedSearchCursor, error) {
	if value == "" {
		return combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchContent}, nil
	}
	if len(value) > maxCombinedCursorBytes {
		return combinedSearchCursor{}, invalidCombinedSearchCursor{}
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var state combinedSearchCursor
	if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.Fingerprint != combinedSearchFingerprint(input) || (state.Phase != combinedSearchContent && state.Phase != combinedSearchDedicated) || state.Checksum != combinedSearchChecksum(state) || (state.Phase == combinedSearchContent && state.Source == "") {
		return combinedSearchCursor{}, invalidCombinedSearchCursor{}
	}
	return state, nil
}

func encodeCombinedSearchCursor(state combinedSearchCursor) string {
	state.Checksum = combinedSearchChecksum(state)
	data, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(data)
}

func combinedSearchFingerprint(input searchaction.Input) string {
	input.Cursor = ""
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func combinedSearchChecksum(state combinedSearchCursor) string {
	state.Checksum = ""
	data, _ := json.Marshal(state)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type catalogGlobalSearchSource struct {
	store *catalog.Store
}

func (s catalogGlobalSearchSource) Search(ctx context.Context, input searchaction.Input) (searchaction.Result, error) {
	types, err := searchaction.Types(input.Type)
	if err != nil {
		return searchaction.Result{}, err
	}
	lister := &catalogSearchLister{store: s.store, environment: input.Environment, site: input.Site, skipUnavailable: input.Type == "", observations: make(map[string]catalogSearchObservation)}
	adapter := resourcesearch.NewAdapter(lister)
	page, err := adapter.Search(ctx, resourcesearch.Input{Types: types, Terms: input.Terms, ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return searchaction.Result{}, err
	}
	var generation *searchaction.Generation
	shared := len(lister.observations) > 0
	for _, observation := range lister.observations {
		after, err := s.store.ReadResources(ctx, observation.query)
		if err != nil || catalogResourceFingerprint(after) != catalogResourceFingerprint(observation.result) {
			return searchaction.Result{}, invalidCatalogResourceCursor{}
		}
		result := observation.result
		if result.Coverage != "complete" || result.GenerationID == "" {
			shared = false
			continue
		}
		if generation == nil {
			generation = &searchaction.Generation{ID: result.GenerationID, Environment: input.Environment, Site: input.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Stale: result.Stale}
		} else if generation.ID != result.GenerationID {
			shared = false
		}
	}
	if shared {
		return searchResult(page, generation), nil
	}
	page.Warnings = append(page.Warnings, "Search used partial catalog records or independently refreshed resource snapshots; no shared complete generation describes this page.")
	return searchResult(page, nil), nil
}

type catalogSearchObservation struct {
	query  catalog.ResourceQuery
	result catalog.ResourceResult
}

type catalogSearchLister struct {
	store             *catalog.Store
	environment, site string
	skipUnavailable   bool
	observations      map[string]catalogSearchObservation
}

func (s *catalogSearchLister) List(ctx context.Context, resourceType, cursor string, limit int) (resourcesearch.Page, error) {
	state, err := decodeCatalogResourceCursor(cursor)
	if err != nil {
		return resourcesearch.Page{}, err
	}
	query := catalog.ResourceQuery{Environment: s.environment, Site: s.site, Kind: resourceType, Offset: state.Offset, Limit: limit}
	result, err := s.store.ReadResources(ctx, query)
	if err != nil {
		var unavailable interface{ CatalogScopeUnavailable() bool }
		var uninitialized interface{ CatalogUninitialized() bool }
		missing := (errors.As(err, &unavailable) && unavailable.CatalogScopeUnavailable()) || (errors.As(err, &uninitialized) && uninitialized.CatalogUninitialized())
		if missing {
			if s.skipUnavailable {
				return resourcesearch.Page{Warnings: []string{"Catalog does not contain " + resourceType + " resources; that type was omitted."}}, nil
			}
			return resourcesearch.Page{}, catalogSearchScopeUnavailable{resourceType: resourceType, cause: err}
		}
		return resourcesearch.Page{}, err
	}
	fingerprint := catalogResourceFingerprint(result)
	if s.observations != nil {
		if previous, ok := s.observations[resourceType]; ok && catalogResourceFingerprint(previous.result) != fingerprint {
			return resourcesearch.Page{}, invalidCatalogResourceCursor{}
		}
		s.observations[resourceType] = catalogSearchObservation{query: query, result: result}
	}
	if state.Fingerprint != "" && state.Fingerprint != fingerprint {
		return resourcesearch.Page{}, invalidCatalogResourceCursor{}
	}
	items := make([]resourcesearch.Item, len(result.Entries))
	for index, item := range result.Entries {
		items[index] = resourcesearch.Item{LUID: item.LUID, Type: item.Kind, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner}
	}
	next := ""
	if state.Offset+len(result.Entries) < result.Total {
		next = encodeCatalogResourceCursor(catalogResourceCursor{Offset: state.Offset + len(result.Entries), Fingerprint: fingerprint})
	}
	return resourcesearch.Page{Items: items, NextCursor: next, Total: result.Total}, nil
}

type catalogSearchScopeUnavailable struct {
	resourceType string
	cause        error
}

func (e catalogSearchScopeUnavailable) Error() string {
	return fmt.Sprintf("catalog does not contain %s resources", e.resourceType)
}
func (e catalogSearchScopeUnavailable) Unwrap() error               { return e.cause }
func (catalogSearchScopeUnavailable) CatalogScopeUnavailable() bool { return true }

type catalogResourceCursor struct {
	Offset      int    `json:"o"`
	Fingerprint string `json:"f"`
}

type invalidCatalogResourceCursor struct{}

func (invalidCatalogResourceCursor) Error() string {
	return "catalog resource cursor is invalid or stale"
}
func (invalidCatalogResourceCursor) InvalidSearchCursor() bool { return true }

func decodeCatalogResourceCursor(value string) (catalogResourceCursor, error) {
	if value == "" {
		return catalogResourceCursor{}, nil
	}
	if len(value) > 4096 {
		return catalogResourceCursor{}, invalidCatalogResourceCursor{}
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor catalogResourceCursor
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Offset < 1 || cursor.Fingerprint == "" {
		return catalogResourceCursor{}, invalidCatalogResourceCursor{}
	}
	return cursor, nil
}

func encodeCatalogResourceCursor(cursor catalogResourceCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func catalogResourceFingerprint(result catalog.ResourceResult) string {
	data, _ := json.Marshal(struct {
		Total          int    `json:"total"`
		Coverage       string `json:"coverage"`
		GenerationID   string `json:"generation_id"`
		GeneratedAt    string `json:"generated_at"`
		NewestObserved string `json:"newest_observed"`
	}{result.Total, result.Coverage, result.GenerationID, result.GeneratedAt.UTC().Format(time.RFC3339Nano), result.NewestObserved.UTC().Format(time.RFC3339Nano)})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func searchResult(page resourcesearch.Page, generation *searchaction.Generation) searchaction.Result {
	items := make([]searchaction.Item, len(page.Items))
	for index, item := range page.Items {
		items[index] = searchaction.Item{LUID: item.LUID, Type: item.Type, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner, ModifiedAt: item.ModifiedAt}
	}
	return searchaction.Result{Items: items, Page: searchaction.Page{NextCursor: page.NextCursor, Total: page.Total, MoreAvailable: page.MoreAvailable, UnresolvedMoreAvailable: page.UnresolvedMoreAvailable}, Warnings: page.Warnings, Generation: generation, Source: page.Source}
}

// completeLiveSearchLister routes blank typed searches through the same
// complete-inventory services as the public list commands.
type completeLiveSearchLister struct {
	environment string
	content     *remoteContentCommands
	admin       *remoteAdminCommands
}

type completeListSearchPager interface {
	searchPage(context.Context, string, string, int, resourcesearch.Input) (resourcesearch.Page, error)
}

type completeLiveSearchAdapter struct{ lister completeListSearchPager }

type completeListSearchCursor struct {
	Version      int    `json:"v"`
	Fingerprint  string `json:"f"`
	TypeIndex    int    `json:"t"`
	SourceCursor string `json:"c,omitempty"`
	SourceLimit  int    `json:"l,omitempty"`
}

type invalidCompleteListSearchCursor struct{}

func (invalidCompleteListSearchCursor) Error() string {
	return "complete list search cursor is invalid"
}
func (invalidCompleteListSearchCursor) InvalidSearchCursor() bool { return true }

func (a *completeLiveSearchAdapter) Search(ctx context.Context, input resourcesearch.Input) (resourcesearch.Page, error) {
	if a == nil || a.lister == nil || len(input.Types) == 0 || input.Limit < 1 || input.Limit > 100 {
		return resourcesearch.Page{}, errors.New("complete live search requires configured list services, types, and a bounded limit")
	}
	types := append([]string(nil), input.Types...)
	if len(types) == 1 {
		return a.lister.searchPage(ctx, types[0], input.Cursor, input.Limit, input)
	}
	sort.Strings(types)
	state := completeListSearchCursor{Version: 1, Fingerprint: completeListSearchFingerprint(input)}
	if input.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if len(input.Cursor) > 12000 || err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.Fingerprint != completeListSearchFingerprint(input) || state.TypeIndex < 0 || state.TypeIndex >= len(types) || (state.SourceCursor != "" && (state.SourceLimit < 1 || state.SourceLimit > input.Limit)) || (state.SourceCursor == "" && state.SourceLimit != 0) {
			return resourcesearch.Page{}, invalidCompleteListSearchCursor{}
		}
	}
	result := resourcesearch.Page{Items: []resourcesearch.Item{}}
	for state.TypeIndex < len(types) && len(result.Items) < input.Limit {
		remaining := input.Limit - len(result.Items)
		sourceLimit := remaining
		if state.SourceCursor != "" {
			sourceLimit = state.SourceLimit
		}
		page, err := a.lister.searchPage(ctx, types[state.TypeIndex], state.SourceCursor, sourceLimit, input)
		if err != nil {
			return resourcesearch.Page{}, err
		}
		if len(page.Items) > remaining {
			return resourcesearch.Page{}, errors.New("complete live search list service exceeded the requested result bound")
		}
		result.UnresolvedMoreAvailable = result.UnresolvedMoreAvailable || page.UnresolvedMoreAvailable || (page.MoreAvailable && page.NextCursor == "")
		result.MoreAvailable = result.UnresolvedMoreAvailable
		result.Items = append(result.Items, page.Items...)
		if len(types) == 1 {
			result.Total = page.Total
		}
		result.Warnings = append(result.Warnings, page.Warnings...)
		if result.Source == "" {
			result.Source = page.Source
		} else if page.Source != "" && result.Source != page.Source {
			result.Source = "mixed"
		}
		if page.TableauRequestID != "" {
			result.TableauRequestID = page.TableauRequestID
		}
		if page.NextCursor != "" {
			state.SourceCursor = page.NextCursor
			state.SourceLimit = sourceLimit
			result.NextCursor = encodeCompleteListSearchCursor(state)
			return result, nil
		}
		state.TypeIndex++
		state.SourceCursor = ""
		state.SourceLimit = 0
	}
	if state.TypeIndex < len(types) {
		result.NextCursor = encodeCompleteListSearchCursor(state)
	}
	return result, nil
}

func completeListSearchFingerprint(input resourcesearch.Input) string {
	input.Cursor = ""
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeCompleteListSearchCursor(state completeListSearchCursor) string {
	data, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(data)
}

func (s *completeLiveSearchLister) searchPage(ctx context.Context, resourceType, cursor string, limit int, searchInput resourcesearch.Input) (resourcesearch.Page, error) {
	if s == nil || s.content == nil || s.admin == nil {
		return resourcesearch.Page{}, errors.New("complete live search list services are not configured")
	}
	switch resourceType {
	case "workbook":
		out, err := s.content.ListWorkbooks(ctx, workbooklist.Input{Environment: s.environment, Cursor: cursor, Limit: limit, ProjectName: searchInput.ProjectPath, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Workbooks))
		for i, item := range out.Workbooks {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "datasource":
		out, err := s.content.ListDatasources(ctx, datasourcelist.Input{Environment: s.environment, Cursor: cursor, Limit: limit, ProjectName: searchInput.ProjectPath, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Datasources))
		for i, item := range out.Datasources {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "flow":
		out, err := s.content.ListFlows(ctx, flowlist.Input{Environment: s.environment, Cursor: cursor, Limit: limit, ProjectName: searchInput.ProjectPath, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Flows))
		for i, item := range out.Flows {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "project":
		out, err := s.content.ListProjects(ctx, projectlist.Input{Environment: s.environment, Cursor: cursor, Limit: limit, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Projects))
		for i, item := range out.Projects {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "user":
		out, err := s.admin.ListAdminUsers(ctx, userlist.Input{Environment: s.environment, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Users))
		for i, item := range out.Users {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "group":
		out, err := s.admin.ListAdminGroups(ctx, adminlist.Input{Environment: s.environment, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Groups))
		for i, item := range out.Groups {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	default:
		return resourcesearch.Page{}, fmt.Errorf("unsupported complete live search type %q", resourceType)
	}
}

func completeListSearchPage(items []resourcesearch.Item, total int, nextCursor string, moreAvailable bool, requestID string, source *readsource.Metadata) resourcesearch.Page {
	page := resourcesearch.Page{Items: items, Total: total, NextCursor: nextCursor, MoreAvailable: moreAvailable, TableauRequestID: requestID}
	if source != nil {
		switch source.Mode {
		case readsource.Tableau:
			page.Source = "live"
		case readsource.Catalog:
			page.Source = "catalog"
		}
		if source.CatalogWarning != "" {
			page.Warnings = []string{source.CatalogWarning}
		}
	}
	return page
}

type liveSearchLister struct {
	environment, site string
	workbooks         workbooklist.Reader
	datasources       datasourcelist.Reader
	flows             flowlist.Reader
	projects          projectlist.Reader
	users             userlist.Reader
	groups            adminlist.Reader
	pulse             *tableaupulse.Client
	definitionPages   map[string]tableaupulse.DefinitionPage
}

func newLiveSearchLister(connection authenticatedTableau) (*liveSearchLister, error) {
	projectClient := tableauproject.NewClient(connection.transport, connection.session, connection.environment.URL)
	projects := resourceproject.NewAdapter(projectClient)
	datasourceClient := tableaudatasource.NewClient(connection.transport, connection.session, connection.environment.URL)
	flowClient := tableauflow.NewClient(connection.transport, connection.session, connection.environment.URL)
	adminClient := tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL)
	pulseClient, err := tableaupulse.NewClient(connection.transport, connection.session, connection.environment.URL)
	if err != nil {
		return nil, fmt.Errorf("configure authenticated Pulse search client: %w", err)
	}
	return &liveSearchLister{
		environment:     connection.environment.Alias,
		site:            connection.environment.SiteContentURL,
		workbooks:       workbookListReader{adapter: resourceworkbook.NewAdapterWithProjectResolver(tableauworkbook.NewClient(connection.transport, connection.session, connection.environment.URL), projects)},
		datasources:     datasourceListReader{adapter: resourcedatasource.NewAdapterWithProjectResolver(datasourceClient, projects), projects: projects},
		flows:           flowListReader{adapter: resourceflow.NewAdapter(flowClient, projects)},
		projects:        projectListReader{adapter: projects},
		users:           adminUserListReader{adapter: resourceadmin.NewAdapter(adminClient)},
		groups:          adminGroupListReader{adapter: resourceadmin.NewAdapter(adminClient)},
		pulse:           pulseClient,
		definitionPages: make(map[string]tableaupulse.DefinitionPage),
	}, nil
}

func (s *liveSearchLister) List(ctx context.Context, resourceType, cursor string, limit int) (resourcesearch.Page, error) {
	switch resourceType {
	case "workbook":
		out, err := workbooklist.New(s.workbooks).Execute(ctx, workbooklist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Workbooks))
		for i, item := range out.Workbooks {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "datasource":
		out, err := datasourcelist.New(s.datasources).Execute(ctx, datasourcelist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Datasources))
		for i, item := range out.Datasources {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "flow":
		out, err := flowlist.New(s.flows).Execute(ctx, flowlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Flows))
		for i, item := range out.Flows {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "project":
		out, err := projectlist.New(s.projects).Execute(ctx, projectlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Projects))
		for i, item := range out.Projects {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "user":
		out, err := userlist.New(s.users).Execute(ctx, userlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Users))
		for i, item := range out.Users {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "group":
		out, err := adminlist.New(s.groups).Execute(ctx, adminlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Groups))
		for i, item := range out.Groups {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "definition":
		out, err := definitionlist.New(&pulseDefinitionListAdapter{client: s.pulse}).Execute(ctx, definitionlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Definitions))
		for i, item := range out.Definitions {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "metric":
		return s.listMetrics(ctx, cursor, limit)
	default:
		return resourcesearch.Page{}, fmt.Errorf("unsupported search type %q", resourceType)
	}
}

type metricSearchCursor struct {
	Version             int    `json:"v"`
	DefinitionPageToken string `json:"d,omitempty"`
	DefinitionIndex     int    `json:"i,omitempty"`
	MetricPageToken     string `json:"m,omitempty"`
	DefinitionDigest    string `json:"s,omitempty"`
}

func (s *liveSearchLister) listMetrics(ctx context.Context, encoded string, limit int) (resourcesearch.Page, error) {
	state := metricSearchCursor{Version: 1}
	if encoded != "" {
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.DefinitionIndex < 0 || (state.DefinitionDigest == "" && (state.DefinitionIndex != 0 || state.MetricPageToken != "")) {
			return resourcesearch.Page{}, errors.New("invalid Pulse metric search cursor")
		}
	}
	definitions, ok := s.definitionPages[state.DefinitionPageToken]
	if !ok {
		var err error
		definitions, err = s.pulse.ListDefinitions(ctx, tableaupulse.PageRequest{PageSize: 100, PageToken: state.DefinitionPageToken})
		if err != nil {
			return resourcesearch.Page{}, err
		}
		s.definitionPages[state.DefinitionPageToken] = definitions
	}
	digest := definitionPageDigest(definitions)
	if state.DefinitionDigest != "" && state.DefinitionDigest != digest {
		return resourcesearch.Page{}, errors.New("Pulse definition inventory changed during metric search")
	}
	state.DefinitionDigest = digest
	if state.DefinitionIndex > len(definitions.Definitions) {
		return resourcesearch.Page{}, errors.New("invalid Pulse metric search cursor")
	}
	if state.DefinitionIndex == len(definitions.Definitions) {
		if definitions.NextPageToken == "" {
			return resourcesearch.Page{Items: []resourcesearch.Item{}}, nil
		}
		state.DefinitionPageToken = definitions.NextPageToken
		state.DefinitionIndex = 0
		state.DefinitionDigest = ""
		return resourcesearch.Page{Items: []resourcesearch.Item{}, NextCursor: encodeMetricSearchCursor(state)}, nil
	}
	definition := definitions.Definitions[state.DefinitionIndex]
	metrics, err := s.pulse.ListMetrics(ctx, definition.LUID, tableaupulse.PageRequest{PageSize: limit, PageToken: state.MetricPageToken})
	if err != nil {
		return resourcesearch.Page{}, err
	}
	result := resourcesearch.Page{Items: make([]resourcesearch.Item, len(metrics.Metrics))}
	for index, metric := range metrics.Metrics {
		name := metric.Name
		if strings.TrimSpace(name) == "" {
			name = definition.Name
		}
		result.Items[index] = resourcesearch.Item{LUID: metric.LUID, Type: "metric", Name: name}
	}
	if metrics.NextPageToken != "" {
		state.MetricPageToken = metrics.NextPageToken
		result.NextCursor = encodeMetricSearchCursor(state)
		return result, nil
	}
	state.DefinitionIndex++
	state.MetricPageToken = ""
	if state.DefinitionIndex < len(definitions.Definitions) {
		result.NextCursor = encodeMetricSearchCursor(state)
	} else if definitions.NextPageToken != "" {
		state.DefinitionPageToken = definitions.NextPageToken
		state.DefinitionIndex = 0
		state.DefinitionDigest = ""
		result.NextCursor = encodeMetricSearchCursor(state)
	}
	return result, nil
}

func definitionPageDigest(page tableaupulse.DefinitionPage) string {
	identities := make([]string, len(page.Definitions))
	for i, item := range page.Definitions {
		identities[i] = item.LUID
	}
	data, _ := json.Marshal(struct {
		Items []string `json:"items"`
		Next  string   `json:"next"`
	}{identities, page.NextPageToken})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeMetricSearchCursor(cursor metricSearchCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}
