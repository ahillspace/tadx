package app

import (
	"context"
	"errors"
	"fmt"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	projectops "github.com/ahillspace/tadx/actions/project"
	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	searchaction "github.com/ahillspace/tadx/actions/search"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/readsource"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourcepulse "github.com/ahillspace/tadx/internal/resources/pulse"
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

// searchProvider constructs command-scoped live and cached search sources.
type searchProvider struct{ runtime *runtimeDependencies }

func newSearchCommands(runtime *runtimeDependencies) *searchaction.Service {
	return searchaction.New(searchProvider{runtime: runtime})
}

func (p searchProvider) CheckCapability(id string) error { return p.runtime.checkManagedCapability(id) }

func (p searchProvider) Target(alias string) (searchaction.Target, error) {
	_, environment, err := p.runtime.environment(alias, false)
	return searchaction.Target{Environment: environment.Alias, Site: environment.SiteContentURL}, err
}

func (p searchProvider) Cached(alias string) (searchaction.Session, error) {
	_, environment, err := p.runtime.environment(alias, false)
	session := searchaction.Session{Target: searchaction.Target{Environment: environment.Alias, Site: environment.SiteContentURL}}
	if err != nil {
		return session, err
	}
	session.Source = resourcesearch.CacheSource{Store: p.runtime.cacheStore(environment)}
	return session, nil
}

func (p searchProvider) Complete(alias string) (searchaction.Session, error) {
	_, environment, err := p.runtime.environment(alias, false)
	session := searchaction.Session{Target: searchaction.Target{Environment: environment.Alias, Site: environment.SiteContentURL}}
	if err != nil {
		return session, err
	}
	lister := &completeLiveSearchLister{
		environment: environment.Alias,
		content:     newRemoteContentCommands(p.runtime),
		admin:       newRemoteAdminCommands(p.runtime),
	}
	lister.projects = projectops.New(projectops.Ports{Provider: projectProvider{commands: lister.content}})
	lister.workbooks = workbookops.New(workbookops.Ports{Read: workbookReadProvider{commands: lister.content}})
	lister.datasources = datasourceops.New(datasourceops.Ports{Read: &datasourceReadProvider{commands: lister.content}})
	lister.flows = flowops.New(flowops.Ports{Read: flowReadProvider{commands: lister.content}})
	session.Source = searchaction.LiveSource{Lists: &searchaction.CompleteLists{Lister: lister}}
	return session, nil
}

func (p searchProvider) Live(ctx context.Context, alias string) (searchaction.Session, error) {
	connection, err := p.runtime.tableauConnection(ctx, alias, false)
	session := searchaction.Session{Target: searchaction.Target{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}}
	if err != nil {
		return session, err
	}
	lister, err := newLiveSearchLister(connection, p.runtime.checkManagedCapability)
	if err != nil {
		return session, err
	}
	nativeClient, err := tableausearch.NewClient(connection.transport, connection.session, connection.environment.URL)
	if err != nil {
		return session, err
	}
	datasourceResolver := tableaudatasource.NewClient(connection.transport, connection.session, connection.environment.URL)
	session.Source = searchaction.LiveSource{Native: resourcesearch.NewNativeAdapter(nativeClient, datasourceResolver), Dedicated: resourcesearch.NewAdapter(lister)}
	return session, nil
}

// completeLiveSearchLister routes blank typed searches through the same
// complete-inventory services as the public list commands.
type completeLiveSearchLister struct {
	environment string
	content     *remoteContentCommands
	projects    *projectops.Service
	admin       *remoteAdminCommands
	workbooks   *workbookops.Service
	datasources *datasourceops.Service
	flows       *flowops.Service
}

func (s *completeLiveSearchLister) SearchPage(ctx context.Context, resourceType, cursor string, limit int, searchInput resourcesearch.Input) (resourcesearch.Page, error) {
	if s == nil || s.content == nil || s.admin == nil {
		return resourcesearch.Page{}, errors.New("complete live search list services are not configured")
	}
	switch resourceType {
	case "workbook":
		out, err := s.workbooks.ListWorkbooks(ctx, workbookops.ListInput{Environment: s.environment, Cursor: cursor, Limit: limit, ProjectName: searchInput.ProjectPath, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Workbooks))
		for i, item := range out.Workbooks {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "datasource":
		out, err := s.datasources.ListDatasources(ctx, datasourceops.ListInput{Environment: s.environment, Cursor: cursor, Limit: limit, ProjectName: searchInput.ProjectPath, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Datasources))
		for i, item := range out.Datasources {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "flow":
		out, err := s.flows.ListFlows(ctx, flowops.ListInput{Environment: s.environment, Cursor: cursor, Limit: limit, ProjectName: searchInput.ProjectPath, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Flows))
		for i, item := range out.Flows {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "project":
		out, err := s.projects.ListProjects(ctx, projectops.ListInput{Environment: s.environment, Cursor: cursor, Limit: limit, OwnerName: searchInput.Owner})
		items := make([]resourcesearch.Item, len(out.Projects))
		for i, item := range out.Projects {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "user":
		out, err := userops.New(adminUserProvider{commands: s.admin}).ListAdminUsers(ctx, userops.ListInput{Environment: s.environment, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Users))
		for i, item := range out.Users {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return completeListSearchPage(items, out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable, out.RequestID, out.Source), err
	case "group":
		out, err := groupops.New(adminGroupProvider{commands: s.admin}).ListAdminGroups(ctx, groupops.ListInput{Environment: s.environment, Cursor: cursor, Limit: limit})
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
		case readsource.Cache:
			page.Source = "cache"
		}
		if source.CacheWarning != "" {
			page.Warnings = []string{source.CacheWarning}
		}
	}
	return page
}

type liveSearchLister struct {
	environment, site string
	workbooks         workbookops.ListReader
	datasources       datasourceops.ListReader
	flows             flowops.ListReader
	projects          projectops.ListReader
	users             userops.ListReader
	groups            groupops.ListReader
	pulse             *tableaupulse.Client
	metrics           *resourcepulse.MetricSearch
}

func newLiveSearchLister(connection authenticatedTableau, checks ...func(string) error) (*liveSearchLister, error) {
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
		environment: connection.environment.Alias,
		site:        connection.environment.SiteContentURL,
		workbooks:   resourceworkbook.ReadPorts{Adapter: resourceworkbook.NewAdapterWithProjectResolver(tableauworkbook.NewClient(connection.transport, connection.session, connection.environment.URL), projects)},
		datasources: resourcedatasource.ReadPorts{Adapter: resourcedatasource.NewAdapterWithProjectResolver(datasourceClient, projects), Projects: resourceproject.NewDiscoveryPaths(projects)},
		flows:       resourceflow.ReadPorts{Adapter: resourceflow.NewAdapter(flowClient, projects)},
		projects:    resourceproject.ListPort{Adapter: projects},
		users:       resourceadmin.UserPorts{Adapter: resourceadmin.NewAdapter(adminClient, checks...)},
		groups:      resourceadmin.GroupPorts{Adapter: resourceadmin.NewAdapter(adminClient, checks...)},
		pulse:       pulseClient,
		metrics:     resourcepulse.NewMetricSearch(pulseClient),
	}, nil
}

func (s *liveSearchLister) List(ctx context.Context, resourceType, cursor string, limit int) (resourcesearch.Page, error) {
	switch resourceType {
	case "workbook":
		out, err := workbookops.List(ctx, s.workbooks, workbookops.ListInput{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Workbooks))
		for i, item := range out.Workbooks {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "datasource":
		out, err := datasourceops.List(ctx, s.datasources, datasourceops.ListInput{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Datasources))
		for i, item := range out.Datasources {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "flow":
		out, err := flowops.List(ctx, s.flows, flowops.ListInput{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Flows))
		for i, item := range out.Flows {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "project":
		out, err := projectops.ListFromReader(ctx, s.projects, projectops.ListInput{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Projects))
		for i, item := range out.Projects {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "user":
		input := userops.ListInput{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit}
		if err := userops.ValidateListInput(&input); err != nil {
			return resourcesearch.Page{}, err
		}
		if err := userops.ValidateListContinuation(&input); err != nil {
			return resourcesearch.Page{}, err
		}
		out, err := userops.List(ctx, s.users, input)
		items := make([]resourcesearch.Item, len(out.Users))
		for i, item := range out.Users {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "group":
		input := groupops.ListInput{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit}
		if err := groupops.ValidateListInput(&input); err != nil {
			return resourcesearch.Page{}, err
		}
		if err := groupops.ValidateListContinuation(&input); err != nil {
			return resourcesearch.Page{}, err
		}
		out, err := groupops.List(ctx, s.groups, input)
		items := make([]resourcesearch.Item, len(out.Groups))
		for i, item := range out.Groups {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable, Total: out.Page.Total}, err
	case "definition":
		input := pulsedefinition.ListInput{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit}
		if err := pulsedefinition.ListValidateInput(&input); err != nil {
			return resourcesearch.Page{}, err
		}
		if err := pulsedefinition.ListValidateContinuation(input); err != nil {
			return resourcesearch.Page{}, err
		}
		out, err := pulsedefinition.List(ctx, &resourcepulse.DefinitionListPort{Client: s.pulse}, input)
		items := make([]resourcesearch.Item, len(out.Definitions))
		for i, item := range out.Definitions {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "metric":
		return s.metrics.List(ctx, cursor, limit)
	default:
		return resourcesearch.Page{}, fmt.Errorf("unsupported search type %q", resourceType)
	}
}
