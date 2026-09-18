package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	datasourceinspect "github.com/ahillspace/tadx/actions/datasource/inspect"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	flowinspect "github.com/ahillspace/tadx/actions/flow/inspect"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	projectinspect "github.com/ahillspace/tadx/actions/project/inspect"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	workbookinspect "github.com/ahillspace/tadx/actions/workbook/inspect"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

func (r *runtimeDependencies) cacheStore(environment config.Environment) *cache.Store {
	return cache.NewTargetStore(filepath.Dir(r.configPath), environment.URL, environment.SiteContentURL, r.now)
}

func (c *remoteContentCommands) cacheStore(alias string) *cache.Store {
	_, environment, _ := c.runtime.environment(alias, false)
	return c.runtime.cacheStore(environment)
}

func (c *remoteContentCommands) resolveCacheTarget(alias string) (string, string, error) {
	_, environment, err := c.runtime.environment(alias, false)
	if err != nil {
		return alias, "", err
	}
	return environment.Alias, environment.SiteContentURL, nil
}

func cacheReadSource(result cache.ResourceResult) *readsource.Metadata {
	observed := result.NewestObserved
	if observed.IsZero() {
		observed = result.GeneratedAt
	}
	value := readsource.Cached(observed, result.Coverage, result.GenerationID, result.GeneratedAt, result.Stale)
	value.CacheWarning = result.InventoryWarning
	return &value
}

func cacheRecordSource(result cache.ResourceResult, entry cache.ResourceEntry) *readsource.Metadata {
	result.Coverage = readsource.CoveragePartial
	if entry.Coverage == "detail" {
		result.Coverage = readsource.CoverageComplete
	}
	source := cacheReadSource(result)
	if result.Coverage == readsource.CoveragePartial {
		source.CoverageReason = "summary_only"
	}
	return source
}

func liveSource(now func() time.Time) *readsource.Metadata {
	value := readsource.Live(now().UTC())
	return &value
}

func cacheReadError(operation, environment, site string, err error) error {
	id, summary := "cache.read_failed", "Cache read failed."
	kind := errs.KindOperation
	var uninitialized interface{ CacheUninitialized() bool }
	var unavailable interface{ CacheScopeUnavailable() bool }
	var missing interface{ CacheResourceNotFound() bool }
	var ambiguous interface{ AmbiguousCacheSelector() bool }
	var invalidCursor interface{ InvalidCacheCursor() bool }
	var refreshRequired interface{ CacheProjectRefreshRequired() bool }
	var schemaRefreshRequired interface{ CacheSchemaRefreshRequired() bool }
	var incompatible interface{ CacheSchemaIncompatible() bool }
	if errors.As(err, &refreshRequired) && refreshRequired.CacheProjectRefreshRequired() {
		return &errs.Error{ID: "cache.project_filter_unavailable", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: "The cache lacks complete project identity coverage for this filter.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Run " + commandhint.Command("cache", "refresh", "--environment", environment, "--scope", "projects,datasources") + ", then repeat the same --cache command."}
	}
	correctiveAction := cacheReadRecovery(operation, environment)
	switch {
	case errors.As(err, &incompatible) && incompatible.CacheSchemaIncompatible():
		id, summary = "cache.schema_incompatible", "The cache schema is inconsistent or unsupported by this build."
		correctiveAction = "Preserve the cache and repair its schema or use a compatible TADX version. For a live answer, explicitly repeat the command without --cache; an identical refresh is not a repair."
	case errors.As(err, &schemaRefreshRequired) && schemaRefreshRequired.CacheSchemaRefreshRequired():
		id, summary = "cache.schema_refresh_required", "The cache schema requires an explicit refresh before cached reads can continue."
		refresh := cacheScopeRefreshCommand(operation, environment)
		if refresh == "" {
			refresh = commandhint.Environment(environment, "cache", "refresh")
		}
		correctiveAction = "Run " + refresh + " to rebuild the cache. Include any other inventory scopes you still need because rebuilding replaces the old cached data. Then repeat the --cache command."
		if operation == "datasource.schema" || strings.HasPrefix(operation, "pulse.") {
			correctiveAction = "Run " + refresh + " to rebuild the cache. Include any other inventory scopes you still need because rebuilding replaces the old cached data. Then run this command without --cache to retrieve and cache its projection before retrying the cached read."
		}
	case errors.As(err, &uninitialized) && uninitialized.CacheUninitialized():
		id, summary = "cache.uninitialized", "The cache is not initialized for this environment and site."
	case errors.As(err, &unavailable) && unavailable.CacheScopeUnavailable():
		id, summary = "cache.scope_not_indexed", "The requested resource scope is not indexed in the cache."
	case errors.As(err, &missing) && missing.CacheResourceNotFound():
		id, summary, kind = "cache.record_not_found", "No cache record matched the selector.", errs.KindUsage
	case errors.As(err, &ambiguous) && ambiguous.AmbiguousCacheSelector():
		id, summary, kind = "cache.selector_ambiguous", "The cache selector matched more than one record.", errs.KindUsage
		correctiveAction = "Use an exact LUID or a selector that identifies one resource; refreshing the cache does not resolve an ambiguous name or path."
	case errors.As(err, &invalidCursor) && invalidCursor.InvalidCacheCursor():
		id, summary, kind = "cache.cursor_invalid", "The cache continuation cursor no longer identifies the current snapshot.", errs.KindUsage
		correctiveAction = "Repeat the same --cache command without the legacy cursor to read the current snapshot."
	}
	return &errs.Error{ID: id, Kind: kind, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: err, Retryable: errs.Bool(false), CorrectiveAction: correctiveAction}
}

func cacheReadRecovery(operation, environment string) string {
	live := "Run this command without --cache for a live answer."
	if operation == "datasource.schema" {
		return live + " The live schema read attempts to cache the requested field and table metadata; inventory refresh does not collect datasource schemas."
	}
	if strings.HasPrefix(operation, "pulse.") {
		return live + " The live read attempts to cache the requested Pulse records; inventory refresh does not collect Pulse records."
	}
	if refresh := cacheScopeRefreshCommand(operation, environment); refresh != "" {
		return live + " To refresh the cached inventory, run " + refresh + ". Then repeat the same --cache command. Include other inventory scopes you still need because refresh replaces the current generation."
	}
	return live
}

func cacheScopeRefreshCommand(operation, environment string) string {
	resource, _, _ := strings.Cut(strings.TrimPrefix(operation, "admin."), ".")
	switch resource {
	case "workbook", "datasource", "flow", "project", "user", "group":
		return commandhint.Command("cache", "refresh", "--environment", environment, "--scope", resource+"s")
	default:
		return ""
	}
}

func unsupportedCacheFilters(operation, environment, site string) error {
	return &errs.Error{ID: "cache.filters_unsupported", Kind: errs.KindUsage, Operation: operation, Environment: environment, Site: site, Summary: "The selected filters are not indexed for this cache read.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --cache to use Tableau filters."}
}

func resourceEntry(environment, site, kind, luid, name, projectPath, owner, coverage string, observedAt time.Time, payload any) (cache.ResourceEntry, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return cache.ResourceEntry{}, err
	}
	projectLUID := ""
	switch item := payload.(type) {
	case workbookinspect.Workbook:
		projectLUID = item.ProjectLUID
	case datasourceinspect.Datasource:
		projectLUID = item.ProjectLUID
	case flowinspect.Flow:
		projectLUID = item.ProjectLUID
	}
	return cache.ResourceEntry{ProjectLUID: projectLUID, Environment: environment, Site: site, Kind: kind, LUID: luid, Name: name, ProjectPath: projectPath, Owner: owner, Payload: encoded, Coverage: coverage, ObservedAt: observedAt}, nil
}

func writeThrough(store *cache.Store, entries []cache.ResourceEntry) {
	if store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = store.UpsertResources(ctx, entries)
	}
}

type cacheWorkbookListReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheWorkbookListReader) ListWorkbooks(ctx context.Context, input workbooklist.PageRequest) (workbooklist.Page, error) {
	if input.OwnerName != "" || input.ProjectName != "" || input.Tag != "" {
		return workbooklist.Page{}, unsupportedCacheFilters("workbook.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "workbook", Name: input.Name, ProjectLUID: input.ProjectLUID, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return workbooklist.Page{}, cacheReadError("workbook.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]workbooklist.Workbook, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = workbooklist.Workbook{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}
	}
	return workbooklist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Workbooks: items, SnapshotCursor: result.NextCursor}, nil
}

type cacheWorkbookGetResolver struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheWorkbookGetResolver) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookinspect.Workbook, error) {
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "workbook", LUID: string(selector.LUID), Name: selector.Name, ProjectLUID: string(selector.ProjectLUID), ProjectPath: selector.ProjectPath, Limit: 2})
	if err != nil {
		return workbookinspect.Workbook{}, cacheReadError("workbook.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = cacheRecordSource(result, entry)
	var item workbookinspect.Workbook
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return workbookinspect.Workbook{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}

type cacheDatasourceListReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheDatasourceListReader) ListDatasources(ctx context.Context, input datasourcelist.PageRequest) (datasourcelist.Page, error) {
	if input.OwnerName != "" || input.Type != "" || input.Tag != "" || input.UpdatedAfter != "" || input.UpdatedBefore != "" {
		return datasourcelist.Page{}, unsupportedCacheFilters("datasource.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "datasource", Name: input.Name, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return datasourcelist.Page{}, cacheReadError("datasource.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]datasourcelist.Datasource, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) == 0 || json.Unmarshal(entry.Payload, &items[index]) != nil {
			items[index] = datasourcelist.Datasource{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
		}
		items[index].ProjectPath = entry.ProjectPath
		if input.ProjectName != "" {
			items[index].ProjectName = input.ProjectName
		}
	}
	return datasourcelist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Datasources: items, SnapshotCursor: result.NextCursor}, nil
}

type cacheDatasourceGetResolver struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheDatasourceGetResolver) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourceinspect.Datasource, error) {
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "datasource", LUID: string(selector.LUID), Name: selector.Name, ProjectLUID: string(selector.ProjectLUID), ProjectPath: selector.ProjectPath, Limit: 2})
	if err != nil {
		return datasourceinspect.Datasource{}, cacheReadError("datasource.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = cacheRecordSource(result, entry)
	var item datasourceinspect.Datasource
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return datasourceinspect.Datasource{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}

type cacheFlowListReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheFlowListReader) ListFlows(ctx context.Context, input flowlist.PageRequest) (flowlist.Page, error) {
	if input.OwnerName != "" || input.ProjectLUID != "" || input.ProjectName != "" {
		return flowlist.Page{}, unsupportedCacheFilters("flow.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "flow", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return flowlist.Page{}, cacheReadError("flow.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]flowlist.Flow, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) == 0 || json.Unmarshal(entry.Payload, &items[index]) != nil {
			items[index] = flowlist.Flow{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
		}
		items[index].ProjectPath = entry.ProjectPath
	}
	return flowlist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Flows: items, SnapshotCursor: result.NextCursor}, nil
}

type cacheFlowGetResolver struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheFlowGetResolver) ResolveFlow(ctx context.Context, selector identity.Selector) (flowinspect.Flow, error) {
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "flow", LUID: string(selector.LUID), Name: selector.Name, ProjectPath: selector.ProjectPath, Limit: 2})
	if err != nil {
		return flowinspect.Flow{}, cacheReadError("flow.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = cacheRecordSource(result, entry)
	var item flowinspect.Flow
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return flowinspect.Flow{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}

type cacheProjectListReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheProjectListReader) ListProjects(ctx context.Context, input projectlist.PageRequest) (projectlist.Page, error) {
	if input.ParentLUID != "" || input.OwnerName != "" || input.TopLevel != nil {
		return projectlist.Page{}, unsupportedCacheFilters("project.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "project", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return projectlist.Page{}, cacheReadError("project.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]projectlist.Project, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = projectlist.Project{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
	}
	return projectlist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Projects: items, SnapshotCursor: result.NextCursor}, nil
}

func snapshotOffset(pageNumber, pageSize int, cursor string) int {
	if cursor != "" {
		return 0
	}
	return (pageNumber - 1) * pageSize
}

type cacheProjectGetResolver struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cacheProjectGetResolver) ResolveProject(ctx context.Context, selector identity.Selector) (projectinspect.Project, error) {
	path := selector.ProjectPath
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "project", LUID: string(selector.LUID), ProjectPath: path, Limit: 2, ExactlyOne: true})
	if err != nil {
		return projectinspect.Project{}, cacheReadError("project.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = cacheRecordSource(result, entry)
	var item projectinspect.Project
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		item.LUID, item.Name, item.Path, item.OwnerLUID = entry.LUID, entry.Name, entry.ProjectPath, entry.Owner
		return item, nil
	}
	return projectinspect.Project{LUID: entry.LUID, Name: entry.Name, Path: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}
