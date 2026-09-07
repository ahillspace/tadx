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
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

func (c *remoteContentCommands) catalogStore() *catalog.Store {
	return catalog.NewStore(filepath.Dir(c.runtime.configPath), c.runtime.now)
}

func (c *remoteContentCommands) resolveCatalogTarget(alias string) (string, string, error) {
	_, environment, err := c.runtime.environment(alias, false)
	if err != nil {
		return alias, "", err
	}
	return environment.Alias, environment.SiteContentURL, nil
}

func catalogReadSource(result catalog.ResourceResult) *readsource.Metadata {
	observed := result.NewestObserved
	if observed.IsZero() {
		observed = result.GeneratedAt
	}
	value := readsource.Cached(observed, result.Coverage, result.GenerationID, result.GeneratedAt, result.Stale)
	value.CatalogWarning = result.InventoryWarning
	return &value
}

func catalogRecordSource(result catalog.ResourceResult, entry catalog.ResourceEntry) *readsource.Metadata {
	result.Coverage = readsource.CoveragePartial
	if entry.Coverage == "detail" {
		result.Coverage = readsource.CoverageComplete
	}
	return catalogReadSource(result)
}

func liveSource(now func() time.Time) *readsource.Metadata {
	value := readsource.Live(now().UTC())
	return &value
}

func catalogReadError(operation, environment, site string, err error) error {
	id, summary := "catalog.read_failed", "Catalog read failed."
	kind := errs.KindOperation
	var uninitialized interface{ CatalogUninitialized() bool }
	var unavailable interface{ CatalogScopeUnavailable() bool }
	var missing interface{ CatalogResourceNotFound() bool }
	var ambiguous interface{ AmbiguousCatalogSelector() bool }
	var invalidCursor interface{ InvalidCatalogCursor() bool }
	switch {
	case errors.As(err, &uninitialized) && uninitialized.CatalogUninitialized():
		id, summary = "catalog.uninitialized", "The catalog is not initialized for this environment and site."
	case errors.As(err, &unavailable) && unavailable.CatalogScopeUnavailable():
		id, summary = "catalog.scope_not_indexed", "The requested resource scope is not indexed in the catalog."
	case errors.As(err, &missing) && missing.CatalogResourceNotFound():
		id, summary, kind = "catalog.record_not_found", "No catalog record matched the selector.", errs.KindUsage
	case errors.As(err, &ambiguous) && ambiguous.AmbiguousCatalogSelector():
		id, summary, kind = "catalog.selector_ambiguous", "The catalog selector matched more than one record.", errs.KindUsage
	case errors.As(err, &invalidCursor) && invalidCursor.InvalidCatalogCursor():
		id, summary, kind = "catalog.cursor_invalid", "The catalog continuation cursor no longer identifies the current snapshot.", errs.KindUsage
	}
	return &errs.Error{ID: id, Kind: kind, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --catalog to query Tableau and update the catalog."}
}

func unsupportedCatalogFilters(operation, environment, site string) error {
	return &errs.Error{ID: "catalog.filters_unsupported", Kind: errs.KindUsage, Operation: operation, Environment: environment, Site: site, Summary: "The selected filters are not indexed for this catalog read.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --catalog to use Tableau filters."}
}

func resourceEntry(environment, site, kind, luid, name, projectPath, owner, coverage string, observedAt time.Time, payload any) (catalog.ResourceEntry, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return catalog.ResourceEntry{}, err
	}
	return catalog.ResourceEntry{Environment: environment, Site: site, Kind: kind, LUID: luid, Name: name, ProjectPath: projectPath, Owner: owner, Payload: encoded, Coverage: coverage, ObservedAt: observedAt}, nil
}

func writeThrough(store *catalog.Store, entries []catalog.ResourceEntry) {
	if store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = store.UpsertResources(ctx, entries)
	}
}

type catalogWorkbookListReader struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogWorkbookListReader) ListWorkbooks(ctx context.Context, input workbooklist.PageRequest) (workbooklist.Page, error) {
	if input.OwnerName != "" || input.ProjectName != "" || input.Tag != "" {
		return workbooklist.Page{}, unsupportedCatalogFilters("workbook.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "workbook", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return workbooklist.Page{}, catalogReadError("workbook.list", r.environment, r.site, err)
	}
	r.source = catalogReadSource(result)
	items := make([]workbooklist.Workbook, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = workbooklist.Workbook{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}
	}
	return workbooklist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Workbooks: items, SnapshotCursor: result.NextCursor}, nil
}

type catalogWorkbookGetResolver struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogWorkbookGetResolver) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookinspect.Workbook, error) {
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "workbook", LUID: string(selector.LUID), Name: selector.Name, ProjectPath: selector.ProjectPath, Limit: 2})
	if err != nil {
		return workbookinspect.Workbook{}, catalogReadError("workbook.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = catalogRecordSource(result, entry)
	var item workbookinspect.Workbook
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return workbookinspect.Workbook{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}

type catalogDatasourceListReader struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogDatasourceListReader) ListDatasources(ctx context.Context, input datasourcelist.PageRequest) (datasourcelist.Page, error) {
	if input.OwnerName != "" || input.ProjectName != "" || input.Type != "" || input.Tag != "" || input.UpdatedAfter != "" || input.UpdatedBefore != "" {
		return datasourcelist.Page{}, unsupportedCatalogFilters("datasource.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "datasource", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return datasourcelist.Page{}, catalogReadError("datasource.list", r.environment, r.site, err)
	}
	r.source = catalogReadSource(result)
	items := make([]datasourcelist.Datasource, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) == 0 || json.Unmarshal(entry.Payload, &items[index]) != nil {
			items[index] = datasourcelist.Datasource{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
		}
		items[index].ProjectPath = entry.ProjectPath
	}
	return datasourcelist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Datasources: items, SnapshotCursor: result.NextCursor}, nil
}

type catalogDatasourceGetResolver struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogDatasourceGetResolver) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourceinspect.Datasource, error) {
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "datasource", LUID: string(selector.LUID), Name: selector.Name, ProjectPath: selector.ProjectPath, Limit: 2})
	if err != nil {
		return datasourceinspect.Datasource{}, catalogReadError("datasource.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = catalogRecordSource(result, entry)
	var item datasourceinspect.Datasource
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return datasourceinspect.Datasource{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}

type catalogFlowListReader struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogFlowListReader) ListFlows(ctx context.Context, input flowlist.PageRequest) (flowlist.Page, error) {
	if input.OwnerName != "" || input.ProjectLUID != "" || input.ProjectName != "" {
		return flowlist.Page{}, unsupportedCatalogFilters("flow.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "flow", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return flowlist.Page{}, catalogReadError("flow.list", r.environment, r.site, err)
	}
	r.source = catalogReadSource(result)
	items := make([]flowlist.Flow, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) == 0 || json.Unmarshal(entry.Payload, &items[index]) != nil {
			items[index] = flowlist.Flow{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
		}
		items[index].ProjectPath = entry.ProjectPath
	}
	return flowlist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Flows: items, SnapshotCursor: result.NextCursor}, nil
}

type catalogFlowGetResolver struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogFlowGetResolver) ResolveFlow(ctx context.Context, selector identity.Selector) (flowinspect.Flow, error) {
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "flow", LUID: string(selector.LUID), Name: selector.Name, ProjectPath: selector.ProjectPath, Limit: 2})
	if err != nil {
		return flowinspect.Flow{}, catalogReadError("flow.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = catalogRecordSource(result, entry)
	var item flowinspect.Flow
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return flowinspect.Flow{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}

type catalogProjectListReader struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogProjectListReader) ListProjects(ctx context.Context, input projectlist.PageRequest) (projectlist.Page, error) {
	if input.ParentLUID != "" || input.OwnerName != "" || input.TopLevel != nil {
		return projectlist.Page{}, unsupportedCatalogFilters("project.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "project", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return projectlist.Page{}, catalogReadError("project.list", r.environment, r.site, err)
	}
	r.source = catalogReadSource(result)
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

type catalogProjectGetResolver struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogProjectGetResolver) ResolveProject(ctx context.Context, selector identity.Selector) (projectinspect.Project, error) {
	path := selector.ProjectPath
	name := ""
	if path != "" {
		parts := strings.Split(path, "/")
		name = parts[len(parts)-1]
	}
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "project", LUID: string(selector.LUID), Name: name, ProjectPath: path, Limit: 2})
	if err != nil {
		return projectinspect.Project{}, catalogReadError("project.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = catalogRecordSource(result, entry)
	var item projectinspect.Project
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		item.LUID, item.Name, item.Path, item.OwnerLUID = entry.LUID, entry.Name, entry.ProjectPath, entry.Owner
		return item, nil
	}
	return projectinspect.Project{LUID: entry.LUID, Name: entry.Name, Path: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}
