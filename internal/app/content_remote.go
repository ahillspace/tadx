package app

import (
	"context"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	lineagepull "github.com/ahillspace/tadx/actions/lineage"
	projectops "github.com/ahillspace/tadx/actions/project"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

type remoteContentCommands struct{ runtime *runtimeDependencies }

func newRemoteContentCommands(runtime *runtimeDependencies) *remoteContentCommands {
	return &remoteContentCommands{runtime: runtime}
}

func (c *remoteContentCommands) dependencies() *contentcli.Dependencies {
	projects := projectops.New(projectops.Ports{Provider: projectProvider{commands: c}})
	mutations := contentMutationProvider{commands: c}
	workbooks := workbookops.New(workbookops.Ports{Mutation: mutations, Read: workbookReadProvider{commands: c}})
	datasources := datasourceops.New(datasourceops.Ports{Mutation: mutations, Read: &datasourceReadProvider{commands: c}, Schema: datasourceSchemaProvider{commands: c}, Pull: datasourcePullProvider{commands: c}, Publish: datasourcePublishProvider{commands: c}})
	flows := flowops.New(flowops.Ports{Mutation: mutations, Read: flowReadProvider{commands: c}, Pull: flowPullProvider{commands: c}, Publish: flowPublishProvider{commands: c}})
	return &contentcli.Dependencies{
		WorkbookLister: workbooks, WorkbookInspector: workbooks, WorkbookDeleter: workbooks, WorkbookMover: workbooks, WorkbookUpdater: workbooks,
		DatasourceLister: datasources, DatasourceInspector: datasources, DatasourceSchema: datasources, DatasourcePuller: datasources, DatasourcePublisher: datasources, DatasourceDeleter: datasources, DatasourceMover: datasources, DatasourceUpdater: datasources,
		ProjectLister: projects, ProjectInspector: projects, ProjectCreator: projects, ProjectUpdater: projects, ProjectDeleter: projects, ProjectMover: projects,
		FlowLister: flows, FlowInspector: flows, FlowPuller: flows, FlowPublisher: flows, FlowMover: flows, FlowDeleter: flows, FlowUpdater: flows,
	}
}

type remoteConnection struct {
	metadataAssets          *metadataassets.Client
	environment             config.Environment
	siteLUID                string
	projects                *resourceproject.Adapter
	projectChanges          resourceproject.MutationClient
	flows                   *resourceflow.Adapter
	flowChanges             *resourceflow.MutationAdapter
	flowNativeChanges       resourceflow.FlowChanges
	lineage                 *resourcelineage.Adapter
	workbooks               *resourceworkbook.Adapter
	workbookChanges         resourceworkbook.WorkbookChanges
	datasources             *resourcedatasource.Adapter
	datasourceChanges       *resourcedatasource.MutationAdapter
	datasourceNativeChanges resourcedatasource.DatasourceChanges
	inventory               tableaucache.Executor
}

func (c *remoteContentCommands) connect(ctx context.Context, alias string, explicit bool) (remoteConnection, error) {
	connection, err := c.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return remoteConnection{environment: connection.environment}, err
	}
	clients := c.runtime.clients(connection)
	projectClient := clients.projects
	projects := resourceproject.NewAdapter(projectClient)
	var paths resourceworkbook.ProjectPathResolver = projects
	if !explicit {
		paths = c.runtime.discoveryPaths(connection)
	}
	workbooks := resourceworkbook.NewAdapterWithProjectResolver(clients.workbooks, paths)
	if explicit {
		workbooks = resourceworkbook.NewAdapterWithProjectIdentityResolver(clients.workbooks, projects)
	}
	flowClient, datasourceClient := clients.flows, clients.datasources
	return remoteConnection{
		metadataAssets:          clients.metadataAssets,
		environment:             connection.environment,
		siteLUID:                connection.session.SiteLUID(),
		projects:                projects,
		projectChanges:          projectClient,
		flows:                   resourceflow.NewAdapter(flowClient, paths),
		flowChanges:             resourceflow.NewMutationAdapter(flowClient),
		flowNativeChanges:       flowClient,
		lineage:                 resourcelineage.NewAdapter(clients.metadata),
		workbooks:               workbooks,
		workbookChanges:         clients.workbooks,
		datasources:             resourcedatasource.NewAdapterWithProjectResolver(datasourceClient, paths),
		datasourceChanges:       resourcedatasource.NewMutationAdapter(datasourceClient),
		datasourceNativeChanges: datasourceClient,
		inventory:               tableaucache.AuthenticatedExecutor{Transport: connection.transport, Session: connection.session, ServerURL: connection.environment.URL, SiteLUID: connection.session.SiteLUID()},
	}, nil
}

type flowPublishProvider struct{ commands *remoteContentCommands }

func (p flowPublishProvider) OpenFlowPublishSource(ctx context.Context, input flowops.PublishInput) (flowops.PublishInput, flowops.ArtifactReader, string, error) {
	source := resourceflow.PublishSource{Manager: artifact.NewFlowManager(p.commands.runtime.now), Workspace: func(ctx context.Context, selector, alias string) (string, string, error) {
		workspace, err := (&workspaceRuntime{runtime: p.commands.runtime}).resolveForEnvironment(ctx, selector, alias)
		return workspace.Name, workspace.Root, err
	}}
	return source.Open(ctx, input)
}

func (p flowPublishProvider) OpenFlowPublish(ctx context.Context, input flowops.PublishInput, sourcePath string) (flowops.PublishSession, error) {
	connection, err := p.commands.connect(ctx, input.Environment, true)
	if err != nil {
		return flowops.PublishSession{}, remoteSetupError("flow.publish", input.Environment, input.Site, connection.environment, err)
	}
	var lifecycle *jobmonitor.Publication
	ports := resourceflow.PublishPorts{Adapter: connection.flows, Projects: connection.projects, Changes: connection.flowChanges,
		Begin: func(ctx context.Context, request flowops.PublishRequest) error {
			started, _, err := p.commands.runtime.publication(ctx, connection.environment.Alias, "flow", sourcePath, request.ProjectLUID, request.Name)
			if err != nil {
				return err
			}
			lifecycle = started
			return nil
		},
		Progress: func(ctx context.Context, label string) { progress.SetLabel(ctx, label) },
	}
	return flowops.PublishSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Resolver: ports, Preparer: ports,
		Lifecycle: func() flowops.PublishLifecycle { return lifecycle },
	}, nil
}

type lineageWorkspace struct{ runtime *runtimeDependencies }

func (w lineageWorkspace) ResolveLineageWorkspace(ctx context.Context, selector, environment string) (lineagepull.Workspace, error) {
	item, err := (&workspaceRuntime{runtime: w.runtime}).resolveForEnvironment(ctx, selector, environment)
	return lineagepull.Workspace{Root: item.Root, Name: item.Name}, err
}

type lineageProvider struct{ commands *remoteContentCommands }

func (p lineageProvider) OpenLineage(ctx context.Context, environment string) (lineagepull.ReadSession, error) {
	connection, err := p.commands.connect(ctx, environment, false)
	session := lineagepull.ReadSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	if err != nil {
		return session, err
	}
	session.SiteLUID = connection.siteLUID
	session.ServerOrigin, err = artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return session, err
	}
	ports := resourcelineage.ReadPorts{Workbooks: connection.workbooks, Datasources: connection.datasources, Flows: connection.flows, Metadata: connection.lineage, Artifacts: artifact.NewLineageManager(p.commands.runtime.now)}
	session.Resolver, session.Reader, session.Writer, session.Previewer = ports, ports, ports, ports
	return session, nil
}

func remoteSetupError(operation, environment, site string, resolved config.Environment, err error) error {
	environment, site = resolvedTarget(environment, site, resolved)
	return capabilitySetupError(operation+".setup", operation, environment, site, "Tableau operation setup failed.", "Review the selected environment, site, and PAT configuration.", err)
}
