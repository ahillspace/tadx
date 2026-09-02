package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	catalogget "github.com/ahillspace/tadx/actions/catalog/get"
	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	catalogstatus "github.com/ahillspace/tadx/actions/catalog/status"
	corecatalog "github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/config"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

const (
	catalogInventoryPageSize = 1000
	catalogInventoryMaxPages = 1000
)

type catalogProjectPager interface {
	ListProjects(context.Context, resourceproject.ListRequest) (resourceproject.Page, error)
}

type catalogWorkbookPager interface {
	ListWorkbooks(context.Context, tableauworkbook.ListRequest) (resourceworkbook.Page, error)
}

type catalogDatasourcePager interface {
	ListDatasources(context.Context, tableaudatasource.ListRequest) (resourcedatasource.Page, error)
}

type catalogFlowPager interface {
	ListFlows(context.Context, tableauflow.ListRequest) (resourceflow.Page, error)
}

type catalogInventory struct {
	projects    catalogProjectPager
	workbooks   catalogWorkbookPager
	datasources catalogDatasourcePager
	flows       catalogFlowPager
	now         func() time.Time
}

type catalogRemoteInventory struct {
	remote *remoteContentCommands
	now    func() time.Time
}

func (i catalogRemoteInventory) Read(ctx context.Context, input catalogrefresh.Input) (catalogrefresh.Snapshot, error) {
	if i.remote == nil {
		return catalogrefresh.Snapshot{}, errors.New("catalog remote inventory is not configured")
	}
	connection, err := i.remote.connect(ctx, input.Environment, false)
	if err != nil {
		return catalogrefresh.Snapshot{}, remoteSetupError("catalog.refresh", input.Environment, input.Site, connection.environment, err)
	}
	if connection.environment.Alias != input.Environment || connection.environment.SiteContentURL != input.Site {
		return catalogrefresh.Snapshot{}, errors.New("catalog refresh resolved a different remote source")
	}
	return (catalogInventory{projects: connection.projects, workbooks: connection.workbooks, datasources: connection.datasources, flows: connection.flows, now: i.now}).Read(ctx, input)
}

func (i catalogInventory) Read(ctx context.Context, input catalogrefresh.Input) (catalogrefresh.Snapshot, error) {
	selected := make(map[string]bool, len(input.Scopes))
	for _, scope := range input.Scopes {
		selected[scope] = true
	}
	needProjects := selected["projects"] || selected["workbooks"] || selected["datasources"] || selected["flows"]
	var projects []resourceproject.Project
	var err error
	if needProjects {
		projects, err = i.readProjects(ctx)
		if err != nil {
			return catalogrefresh.Snapshot{}, err
		}
	}
	paths, err := catalogProjectPaths(projects)
	if err != nil {
		return catalogrefresh.Snapshot{}, err
	}
	records := make([]catalogrefresh.Record, 0)
	if selected["projects"] {
		for _, item := range projects {
			records = append(records, catalogrefresh.Record{LUID: item.LUID, Kind: "project", Name: item.Name, ProjectPath: paths[item.LUID], Owner: item.OwnerLUID})
		}
	}
	if selected["workbooks"] {
		items, readErr := i.readWorkbooks(ctx)
		if readErr != nil {
			return catalogrefresh.Snapshot{}, readErr
		}
		for _, item := range items {
			path, pathErr := catalogItemProjectPath("workbook", item.LUID, item.ProjectLUID, paths)
			if pathErr != nil {
				return catalogrefresh.Snapshot{}, pathErr
			}
			records = append(records, catalogrefresh.Record{LUID: item.LUID, Kind: "workbook", Name: item.Name, ProjectPath: path, Owner: item.OwnerLUID})
		}
	}
	if selected["datasources"] {
		items, readErr := i.readDatasources(ctx)
		if readErr != nil {
			return catalogrefresh.Snapshot{}, readErr
		}
		for _, item := range items {
			path, pathErr := catalogItemProjectPath("datasource", item.LUID, item.ProjectLUID, paths)
			if pathErr != nil {
				return catalogrefresh.Snapshot{}, pathErr
			}
			records = append(records, catalogrefresh.Record{LUID: item.LUID, Kind: "datasource", Name: item.Name, ProjectPath: path, Owner: item.OwnerLUID})
		}
	}
	if selected["flows"] {
		items, readErr := i.readFlows(ctx)
		if readErr != nil {
			return catalogrefresh.Snapshot{}, readErr
		}
		for _, item := range items {
			path, pathErr := catalogItemProjectPath("flow", item.LUID, item.ProjectLUID, paths)
			if pathErr != nil {
				return catalogrefresh.Snapshot{}, pathErr
			}
			records = append(records, catalogrefresh.Record{LUID: item.LUID, Kind: "flow", Name: item.Name, ProjectPath: path, Owner: item.OwnerLUID})
		}
	}
	sort.Slice(records, func(left, right int) bool {
		if records[left].Kind != records[right].Kind {
			return records[left].Kind < records[right].Kind
		}
		if records[left].Name != records[right].Name {
			return records[left].Name < records[right].Name
		}
		if records[left].ProjectPath != records[right].ProjectPath {
			return records[left].ProjectPath < records[right].ProjectPath
		}
		return records[left].LUID < records[right].LUID
	})
	now := i.now
	if now == nil {
		now = time.Now
	}
	return catalogrefresh.Snapshot{GeneratedAt: now().UTC(), Source: "tableau-rest", Records: records}, nil
}

func (i catalogInventory) readProjects(ctx context.Context) ([]resourceproject.Project, error) {
	if i.projects == nil {
		return nil, errors.New("catalog project inventory is not configured")
	}
	items := make([]resourceproject.Project, 0)
	seen := make(map[string]resourceproject.Project)
	expectedTotal, expectedSize := -1, -1
	for number := 1; number <= catalogInventoryMaxPages; number++ {
		page, err := i.projects.ListProjects(ctx, resourceproject.ListRequest{PageNumber: number, PageSize: catalogInventoryPageSize})
		if err != nil {
			return nil, err
		}
		if err := validateCatalogPage("project", number, page.Number, page.Size, page.Total, len(page.Items), &expectedTotal, &expectedSize); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if current, exists := seen[item.LUID]; exists {
				if !reflect.DeepEqual(current, item) {
					return nil, fmt.Errorf("catalog project inventory returned conflicting LUID %q", item.LUID)
				}
				return nil, fmt.Errorf("catalog project inventory repeated LUID %q", item.LUID)
			}
			seen[item.LUID] = item
			items = append(items, item)
		}
		if number*page.Size >= page.Total {
			if len(items) != page.Total {
				return nil, fmt.Errorf("catalog project inventory returned %d of %d records", len(items), page.Total)
			}
			return items, nil
		}
	}
	return nil, errors.New("catalog project inventory exceeded the page bound")
}

func (i catalogInventory) readWorkbooks(ctx context.Context) ([]resourceworkbook.Workbook, error) {
	if i.workbooks == nil {
		return nil, errors.New("catalog workbook inventory is not configured")
	}
	items := make([]resourceworkbook.Workbook, 0)
	seen := make(map[string]bool)
	expectedTotal, expectedSize := -1, -1
	for number := 1; number <= catalogInventoryMaxPages; number++ {
		page, err := i.workbooks.ListWorkbooks(ctx, tableauworkbook.ListRequest{PageNumber: number, PageSize: catalogInventoryPageSize})
		if err != nil {
			return nil, err
		}
		if err := validateCatalogPage("workbook", number, page.Number, page.Size, page.Total, len(page.Items), &expectedTotal, &expectedSize); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if seen[item.LUID] {
				return nil, fmt.Errorf("catalog workbook inventory repeated LUID %q", item.LUID)
			}
			seen[item.LUID] = true
			items = append(items, item)
		}
		if number*page.Size >= page.Total {
			if len(items) != page.Total {
				return nil, fmt.Errorf("catalog workbook inventory returned %d of %d records", len(items), page.Total)
			}
			return items, nil
		}
	}
	return nil, errors.New("catalog workbook inventory exceeded the page bound")
}

func (i catalogInventory) readDatasources(ctx context.Context) ([]resourcedatasource.Datasource, error) {
	if i.datasources == nil {
		return nil, errors.New("catalog datasource inventory is not configured")
	}
	items := make([]resourcedatasource.Datasource, 0)
	seen := make(map[string]bool)
	expectedTotal, expectedSize := -1, -1
	for number := 1; number <= catalogInventoryMaxPages; number++ {
		page, err := i.datasources.ListDatasources(ctx, tableaudatasource.ListRequest{PageNumber: number, PageSize: catalogInventoryPageSize})
		if err != nil {
			return nil, err
		}
		if err := validateCatalogPage("datasource", number, page.Number, page.Size, page.Total, len(page.Items), &expectedTotal, &expectedSize); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if seen[item.LUID] {
				return nil, fmt.Errorf("catalog datasource inventory repeated LUID %q", item.LUID)
			}
			seen[item.LUID] = true
			items = append(items, item)
		}
		if number*page.Size >= page.Total {
			if len(items) != page.Total {
				return nil, fmt.Errorf("catalog datasource inventory returned %d of %d records", len(items), page.Total)
			}
			return items, nil
		}
	}
	return nil, errors.New("catalog datasource inventory exceeded the page bound")
}

func (i catalogInventory) readFlows(ctx context.Context) ([]resourceflow.Flow, error) {
	if i.flows == nil {
		return nil, errors.New("catalog flow inventory is not configured")
	}
	items := make([]resourceflow.Flow, 0)
	seen := make(map[string]bool)
	expectedTotal, expectedSize := -1, -1
	for number := 1; number <= catalogInventoryMaxPages; number++ {
		page, err := i.flows.ListFlows(ctx, tableauflow.ListRequest{PageNumber: number, PageSize: catalogInventoryPageSize})
		if err != nil {
			return nil, err
		}
		if err := validateCatalogPage("flow", number, page.Number, page.Size, page.Total, len(page.Items), &expectedTotal, &expectedSize); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if seen[item.LUID] {
				return nil, fmt.Errorf("catalog flow inventory repeated LUID %q", item.LUID)
			}
			seen[item.LUID] = true
			items = append(items, item)
		}
		if number*page.Size >= page.Total {
			if len(items) != page.Total {
				return nil, fmt.Errorf("catalog flow inventory returned %d of %d records", len(items), page.Total)
			}
			return items, nil
		}
	}
	return nil, errors.New("catalog flow inventory exceeded the page bound")
}

func validateCatalogPage(kind string, requested, number, size, total, count int, expectedTotal, expectedSize *int) error {
	if number != requested || size <= 0 || size > catalogInventoryPageSize || total < 0 || count > size || (number-1)*size+count > total {
		return fmt.Errorf("catalog %s inventory returned inconsistent pagination", kind)
	}
	if *expectedTotal < 0 {
		*expectedTotal, *expectedSize = total, size
		return nil
	}
	if total != *expectedTotal || size != *expectedSize {
		return fmt.Errorf("catalog %s inventory pagination changed during refresh", kind)
	}
	return nil
}

func catalogProjectPaths(projects []resourceproject.Project) (map[string]string, error) {
	byID := make(map[string]resourceproject.Project, len(projects))
	for _, item := range projects {
		if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" {
			return nil, errors.New("catalog project inventory omitted authoritative identity")
		}
		if _, exists := byID[item.LUID]; exists {
			return nil, fmt.Errorf("catalog project inventory repeated LUID %q", item.LUID)
		}
		byID[item.LUID] = item
	}
	paths := make(map[string]string, len(projects))
	var resolve func(string, map[string]bool) (string, error)
	resolve = func(luid string, visiting map[string]bool) (string, error) {
		if path, exists := paths[luid]; exists {
			return path, nil
		}
		item, exists := byID[luid]
		if !exists {
			return "", fmt.Errorf("catalog project %q references a missing parent", luid)
		}
		if visiting[luid] {
			return "", fmt.Errorf("catalog project hierarchy contains a cycle at %q", luid)
		}
		visiting[luid] = true
		path := item.Name
		if item.ParentLUID != "" {
			parent, err := resolve(item.ParentLUID, visiting)
			if err != nil {
				return "", err
			}
			path = parent + "/" + item.Name
		}
		delete(visiting, luid)
		paths[luid] = path
		return path, nil
	}
	for luid := range byID {
		if _, err := resolve(luid, make(map[string]bool)); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func catalogItemProjectPath(kind, luid, projectLUID string, paths map[string]string) (string, error) {
	if strings.TrimSpace(projectLUID) == "" {
		return "", fmt.Errorf("catalog %s %q omitted its authoritative project LUID", kind, luid)
	}
	path, exists := paths[projectLUID]
	if !exists {
		return "", fmt.Errorf("catalog %s %q references missing project %q", kind, luid, projectLUID)
	}
	return path, nil
}

type catalogGenerationWriter struct{ store *corecatalog.FileStore }

func (w catalogGenerationWriter) Replace(ctx context.Context, input catalogrefresh.Generation) (catalogrefresh.WriteResult, error) {
	records := make([]corecatalog.Record, len(input.Records))
	for index, item := range input.Records {
		records[index] = corecatalog.Record{LUID: item.LUID, Kind: item.Kind, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner}
	}
	result, err := w.store.Replace(ctx, corecatalog.Generation{Environment: input.Environment, Site: input.Site, GeneratedAt: input.GeneratedAt, Complete: input.Complete, Source: input.Source, Scopes: append([]string(nil), input.Scopes...), Records: records})
	return catalogrefresh.WriteResult{GenerationID: result.GenerationID, Path: result.Path, RecordCount: result.RecordCount}, err
}

type catalogStoreGetter struct{ store *corecatalog.FileStore }

func (g catalogStoreGetter) Get(ctx context.Context, input catalogget.Input) (catalogget.Result, error) {
	result, err := g.store.Get(ctx, corecatalog.Lookup{Environment: input.Environment, Site: input.Site, SiteSelected: input.SiteResolved, LUID: input.LUID, Kind: input.Kind, Name: input.Name, ProjectPath: input.ProjectPath})
	if err != nil {
		return catalogget.Result{}, err
	}
	return catalogget.Result{Item: catalogget.Item{LUID: result.Record.LUID, Kind: result.Record.Kind, Name: result.Record.Name, ProjectPath: result.Record.ProjectPath, Owner: result.Record.Owner}, Generation: catalogget.Generation{ID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Stale: result.Stale}, Warnings: append([]string(nil), result.Warnings...)}, nil
}

type catalogStoreStatuser struct{ store *corecatalog.FileStore }

func (s catalogStoreStatuser) Status(ctx context.Context, input catalogstatus.Input) (catalogstatus.Result, error) {
	result, err := s.store.Status(ctx, corecatalog.Selection{Environment: input.Environment, Site: input.Site, SiteSelected: input.SiteResolved})
	if err != nil {
		return catalogstatus.Result{}, err
	}
	return catalogstatus.Result{ID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Age: result.Age.String(), Complete: result.Complete, Stale: result.Stale, Source: result.Source, Path: result.Path, Records: result.RecordCount, Warnings: append([]string(nil), result.Warnings...)}, nil
}

type catalogGroup2Commands struct {
	runtime *runtimeDependencies
	remote  *remoteContentCommands
}

func newCatalogGroup2Commands(runtime *runtimeDependencies, remote *remoteContentCommands) *catalogGroup2Commands {
	return &catalogGroup2Commands{runtime: runtime, remote: remote}
}

func (c *catalogGroup2Commands) refresher() *catalogRefreshService {
	return &catalogRefreshService{commands: c}
}

func (c *catalogGroup2Commands) getter() *catalogGetService {
	return &catalogGetService{commands: c}
}

func (c *catalogGroup2Commands) statuser() *catalogStatusService {
	return &catalogStatusService{commands: c}
}

func (c *catalogGroup2Commands) store() *corecatalog.FileStore {
	return corecatalog.NewFileStore(filepath.Dir(c.runtime.configPath), c.runtime.now)
}

func (c *catalogGroup2Commands) resolve(inputEnvironment, inputSite, operation string) (config.Environment, error) {
	_, environment, err := c.runtime.environment(inputEnvironment, false)
	if err != nil {
		return environment, capabilitySetupError(operation+".setup", operation, inputEnvironment, inputSite, "Catalog operation setup failed.", "Review the selected environment and catalog configuration.", err)
	}
	if inputSite != "" && inputSite != environment.SiteContentURL {
		return environment, capabilitySetupError(operation+".setup", operation, environment.Alias, inputSite, "Catalog source site does not match the selected environment.", "Choose the configured exact site, then retry.", errors.New("catalog source site mismatch"))
	}
	return environment, nil
}

type catalogRefreshService struct{ commands *catalogGroup2Commands }

func (s *catalogRefreshService) Execute(ctx context.Context, input catalogrefresh.Input) (catalogrefresh.Output, error) {
	environment, err := s.commands.resolve(input.Environment, input.Site, "catalog.refresh")
	if err != nil {
		return catalogrefresh.Output{}, err
	}
	input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
	inventory := catalogRemoteInventory{remote: s.commands.remote, now: s.commands.runtime.now}
	return catalogrefresh.New(inventory, catalogGenerationWriter{store: s.commands.store()}).Execute(ctx, input)
}

type catalogGetService struct{ commands *catalogGroup2Commands }

func (s *catalogGetService) Execute(ctx context.Context, input catalogget.Input) (catalogget.Output, error) {
	environment, err := s.commands.resolve(input.Environment, input.Site, "catalog.get")
	if err != nil {
		return catalogget.Output{}, err
	}
	input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
	return catalogget.New(catalogStoreGetter{store: s.commands.store()}).Execute(ctx, input)
}

type catalogStatusService struct{ commands *catalogGroup2Commands }

func (s *catalogStatusService) Execute(ctx context.Context, input catalogstatus.Input) (catalogstatus.Output, error) {
	environment, err := s.commands.resolve(input.Environment, input.Site, "catalog.status")
	if err != nil {
		return catalogstatus.Output{}, err
	}
	input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
	return catalogstatus.New(catalogStoreStatuser{store: s.commands.store()}).Execute(ctx, input)
}
