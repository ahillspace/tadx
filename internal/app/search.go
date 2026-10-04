package app

import (
	"context"
	"fmt"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	projectops "github.com/ahillspace/tadx/actions/project"
	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	searchaction "github.com/ahillspace/tadx/actions/search"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
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
	"github.com/ahillspace/tadx/internal/value"
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
	content := newRemoteContentCommands(p.runtime)
	admin := newRemoteAdminCommands(p.runtime)
	projects := projectops.New(projectops.Ports{Provider: projectProvider{commands: content}})
	workbooks := workbookops.New(workbookops.Ports{Read: workbookReadProvider{commands: content}})
	datasources := datasourceops.New(datasourceops.Ports{Read: &datasourceReadProvider{commands: content}})
	flows := flowops.New(flowops.Ports{Read: flowReadProvider{commands: content}})
	users := userops.New(adminUserProvider{commands: admin})
	groups := groupops.New(adminGroupProvider{commands: admin})
	lists := searchaction.ListSources{
		"workbook": func(ctx context.Context, input value.SearchRequest) (value.SearchPage, error) {
			return workbooks.SearchPage(ctx, environment.Alias, input)
		},
		"datasource": func(ctx context.Context, input value.SearchRequest) (value.SearchPage, error) {
			return datasources.SearchPage(ctx, environment.Alias, input)
		},
		"flow": func(ctx context.Context, input value.SearchRequest) (value.SearchPage, error) {
			return flows.SearchPage(ctx, environment.Alias, input)
		},
		"project": func(ctx context.Context, input value.SearchRequest) (value.SearchPage, error) {
			return projects.SearchPage(ctx, environment.Alias, input)
		},
		"user": func(ctx context.Context, input value.SearchRequest) (value.SearchPage, error) {
			return users.SearchPage(ctx, environment.Alias, input)
		},
		"group": func(ctx context.Context, input value.SearchRequest) (value.SearchPage, error) {
			return groups.SearchPage(ctx, environment.Alias, input)
		},
	}
	session.Source = searchaction.LiveSource{Lists: &searchaction.CompleteLists{Lister: lists}}
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

func newLiveSearchLister(connection authenticatedTableau, checks ...func(string) error) (resourcesearch.ListSources, error) {
	projectClient := tableauproject.NewClient(connection.transport, connection.session, connection.environment.URL)
	projects := resourceproject.NewAdapter(projectClient)
	datasourceClient := tableaudatasource.NewClient(connection.transport, connection.session, connection.environment.URL)
	flowClient := tableauflow.NewClient(connection.transport, connection.session, connection.environment.URL)
	adminClient := tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL)
	pulseClient, err := tableaupulse.NewClient(connection.transport, connection.session, connection.environment.URL)
	if err != nil {
		return nil, fmt.Errorf("configure authenticated Pulse search client: %w", err)
	}
	workbooks := resourceworkbook.ReadPorts{Adapter: resourceworkbook.NewAdapterWithProjectResolver(tableauworkbook.NewClient(connection.transport, connection.session, connection.environment.URL), projects)}
	datasources := resourcedatasource.ReadPorts{Adapter: resourcedatasource.NewAdapterWithProjectResolver(datasourceClient, projects), Projects: resourceproject.NewDiscoveryPaths(projects)}
	flows := resourceflow.ReadPorts{Adapter: resourceflow.NewAdapter(flowClient, projects)}
	projectReader := resourceproject.ListPort{Adapter: projects}
	users := resourceadmin.UserPorts{Adapter: resourceadmin.NewAdapter(adminClient, checks...)}
	groups := resourceadmin.GroupPorts{Adapter: resourceadmin.NewAdapter(adminClient, checks...)}
	definitions := &resourcepulse.DefinitionListPort{Client: pulseClient}
	metrics := resourcepulse.NewMetricSearch(pulseClient)
	environment, site := connection.environment.Alias, connection.environment.SiteContentURL
	return resourcesearch.ListSources{
		"workbook": func(ctx context.Context, cursor string, limit int) (value.SearchPage, error) {
			return workbookops.ListSearch(ctx, workbooks, workbookops.ListInput{Environment: environment, Site: site, Cursor: cursor, Limit: limit})
		},
		"datasource": func(ctx context.Context, cursor string, limit int) (value.SearchPage, error) {
			return datasourceops.ListSearch(ctx, datasources, datasourceops.ListInput{Environment: environment, Site: site, Cursor: cursor, Limit: limit})
		},
		"flow": func(ctx context.Context, cursor string, limit int) (value.SearchPage, error) {
			return flowops.ListSearch(ctx, flows, flowops.ListInput{Environment: environment, Site: site, Cursor: cursor, Limit: limit})
		},
		"project": func(ctx context.Context, cursor string, limit int) (value.SearchPage, error) {
			return projectops.ListSearch(ctx, projectReader, projectops.ListInput{Environment: environment, Site: site, Cursor: cursor, Limit: limit})
		},
		"user": func(ctx context.Context, cursor string, limit int) (value.SearchPage, error) {
			return userops.ListSearch(ctx, users, userops.ListInput{Environment: environment, Site: site, Cursor: cursor, Limit: limit})
		},
		"group": func(ctx context.Context, cursor string, limit int) (value.SearchPage, error) {
			return groupops.ListSearch(ctx, groups, groupops.ListInput{Environment: environment, Site: site, Cursor: cursor, Limit: limit})
		},
		"definition": func(ctx context.Context, cursor string, limit int) (value.SearchPage, error) {
			return pulsedefinition.ListSearch(ctx, definitions, pulsedefinition.ListInput{Environment: environment, Site: site, Cursor: cursor, Limit: limit})
		},
		"metric": metrics.List,
	}, nil
}
