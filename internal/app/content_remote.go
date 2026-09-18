package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	flowdelete "github.com/ahillspace/tadx/actions/flow/delete"
	flowinspect "github.com/ahillspace/tadx/actions/flow/inspect"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	flowmove "github.com/ahillspace/tadx/actions/flow/move"
	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	flowpull "github.com/ahillspace/tadx/actions/flow/pull"
	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	projectdelete "github.com/ahillspace/tadx/actions/project/delete"
	projectinspect "github.com/ahillspace/tadx/actions/project/inspect"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	projectupdate "github.com/ahillspace/tadx/actions/project/update"
	workbookdelete "github.com/ahillspace/tadx/actions/workbook/delete"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cache"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

type remoteContentCommands struct{ runtime *runtimeDependencies }

func newRemoteContentCommands(runtime *runtimeDependencies) *remoteContentCommands {
	return &remoteContentCommands{runtime: runtime}
}

func (c *remoteContentCommands) dependencies() *contentcli.Dependencies {
	return &contentcli.Dependencies{
		WorkbookLister: c, WorkbookInspector: c, WorkbookDeleter: c, WorkbookMover: c, WorkbookUpdater: c,
		DatasourceLister: c, DatasourceInspector: c, DatasourceSchema: c, DatasourcePuller: c, DatasourcePublisher: c, DatasourceDeleter: c, DatasourceMover: c, DatasourceUpdater: c,
		ProjectLister: c, ProjectInspector: c, ProjectCreator: c, ProjectUpdater: c, ProjectDeleter: c, ProjectMover: c,
		FlowLister: c, FlowInspector: c, FlowPuller: c, FlowPublisher: c, FlowMover: c, FlowDeleter: c, FlowUpdater: c,
		LineagePuller: c,
	}
}

type remoteConnection struct {
	metadataAssets    *metadataassets.Client
	environment       config.Environment
	siteLUID          string
	projects          *resourceproject.Adapter
	projectChanges    *resourceproject.MutationAdapter
	flows             *resourceflow.Adapter
	flowChanges       *resourceflow.MutationAdapter
	lineage           *resourcelineage.Adapter
	workbooks         *resourceworkbook.Adapter
	datasources       *resourcedatasource.Adapter
	datasourceChanges *resourcedatasource.MutationAdapter
	inventory         tableaucache.Executor
}

func (c *remoteContentCommands) connect(ctx context.Context, alias string, explicit bool) (remoteConnection, error) {
	connection, err := c.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return remoteConnection{environment: connection.environment}, err
	}
	clients := c.runtime.clients(connection)
	projectClient := clients.projects
	projects := resourceproject.NewAdapter(projectClient)
	var paths resourceworkbook.ProjectPathResolver = workbookProjectResolver{projects}
	if !explicit {
		paths = c.runtime.discoveryPaths(connection)
	}
	flowClient, datasourceClient := clients.flows, clients.datasources
	return remoteConnection{
		metadataAssets:    clients.metadataAssets,
		environment:       connection.environment,
		siteLUID:          connection.session.SiteLUID(),
		projects:          projects,
		projectChanges:    resourceproject.NewMutationAdapter(projectClient),
		flows:             resourceflow.NewAdapter(flowClient, paths),
		flowChanges:       resourceflow.NewMutationAdapter(flowClient),
		lineage:           resourcelineage.NewAdapter(clients.metadata),
		workbooks:         resourceworkbook.NewAdapterWithProjectResolver(clients.workbooks, paths),
		datasources:       resourcedatasource.NewAdapterWithProjectResolver(datasourceClient, paths),
		datasourceChanges: resourcedatasource.NewMutationAdapter(datasourceClient),
		inventory:         cacheTableauExecutor{transport: connection.transport, session: connection.session, serverURL: connection.environment.URL, siteLUID: connection.session.SiteLUID()},
	}, nil
}

func (c *remoteContentCommands) ListProjects(ctx context.Context, input projectlist.Input) (result projectlist.Output, resultErr error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return projectlist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := projectlist.ValidateInput(input); err != nil {
		return projectlist.Output{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return projectlist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheProjectListReader{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := projectlist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauproject.ListFilter(tableauproject.ListRequest{Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel})
	if err != nil {
		return projectlist.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return projectlist.Output{}, remoteSetupError("project.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.cacheStore(input.Environment), tableaucache.ScopeProjects, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CacheMaxConcurrency, Filter: filter})
		if err != nil {
			return projectlist.Output{}, inventoryRefreshError("project.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := projectlist.New(reader).Execute(ctx, input)
		if err != nil {
			return output, err
		}
		if inventory.cacheErr != nil {
			output.Source = inventory.warningSource(observedAt)
			output.Help = append(output.Help, inventory.warningHelp())
		} else if inventory.filtered {
			output.Source = liveSource(c.runtime.now)
		} else {
			output.Source = liveInventorySource(observedAt, inventory.published.GenerationID)
		}
		output.RequestID = finalRequestID(inventory.requestIDs)
		return output, nil
	}
	output, err := projectlist.New(projectListReader{connection.projects}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func projectListIsUnfiltered(input projectlist.Input) bool {
	return input.Name == "" && input.ParentLUID == "" && input.OwnerName == "" && input.TopLevel == nil
}

func (c *remoteContentCommands) InspectProject(ctx context.Context, input projectinspect.Input) (projectinspect.Output, error) {
	if err := projectinspect.ValidateInput(input); err != nil {
		return projectinspect.Output{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return projectinspect.Output{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheProjectGetResolver{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := projectinspect.New(resolver).Execute(ctx, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return projectinspect.Output{}, remoteSetupError("project.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := projectinspect.New(projectGetResolver{connection.projects}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "project", output.Project.LUID, output.Project.Name, output.Project.Path, output.Project.OwnerLUID, "detail", observedAt, output.Project)
	if encodeErr == nil {
		writeThrough(c.cacheStore(input.Environment), []cache.ResourceEntry{entry})
	}
	return output, nil
}

func (c *remoteContentCommands) CreateProject(ctx context.Context, input projectcreate.Input, preview bool) (projectcreate.Output, error) {
	if err := projectcreate.ValidateInput(input); err != nil {
		return projectcreate.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return projectcreate.Output{}, remoteSetupError("project.create", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := projectCreateAdapter{projects: connection.projects, changes: connection.projectChanges, resolved: make(map[string]resourceproject.Project)}
	out, err := projectcreate.New(adapter, adapter).Execute(ctx, input, preview)
	if err == nil && out.Result != nil && out.Result.Project.Path == "" {
		out.Help = append(out.Help, projectMutationPathWarning)
	}
	return out, err
}

func (c *remoteContentCommands) UpdateProject(ctx context.Context, input projectupdate.Input, preview bool) (projectupdate.Output, error) {
	if err := projectupdate.ValidateInput(input); err != nil {
		return projectupdate.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return projectupdate.Output{}, remoteSetupError("project.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := projectUpdateAdapter{projects: connection.projects, changes: connection.projectChanges, resolved: make(map[string]resourceproject.Project)}
	out, err := projectupdate.New(adapter, adapter).Execute(ctx, input, preview)
	if err == nil && out.Result != nil && out.Result.Project.Path == "" {
		out.Help = append(out.Help, projectMutationPathWarning)
	}
	return out, err
}

func (c *remoteContentCommands) DeleteProject(ctx context.Context, input projectdelete.Input, preview bool) (projectdelete.Output, error) {
	if err := projectdelete.ValidateInput(input); err != nil {
		return projectdelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return projectdelete.Output{}, remoteSetupError("project.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := projectDeleteAdapter{projects: connection.projects, changes: connection.projectChanges}
	return projectdelete.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) ListFlows(ctx context.Context, input flowlist.Input) (result flowlist.Output, resultErr error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return flowlist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := flowlist.ValidateInput(input); err != nil {
		return flowlist.Output{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return flowlist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheFlowListReader{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := flowlist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauflow.ListFilter(tableauflow.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName})
	if err != nil {
		return flowlist.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowlist.Output{}, remoteSetupError("flow.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.cacheStore(input.Environment), tableaucache.ScopeFlows, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CacheMaxConcurrency, Filter: filter})
		if err != nil {
			return flowlist.Output{}, inventoryRefreshError("flow.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := flowlist.New(reader).Execute(ctx, input)
		if err != nil {
			return output, err
		}
		if inventory.cacheErr != nil {
			output.Source = inventory.warningSource(observedAt)
			output.Help = append(output.Help, inventory.warningHelp())
		} else if inventory.filtered {
			output.Source = liveSource(c.runtime.now)
		} else {
			output.Source = liveInventorySource(observedAt, inventory.published.GenerationID)
		}
		output.RequestID = finalRequestID(inventory.requestIDs)
		return output, nil
	}
	output, err := flowlist.New(flowListReader{connection.flows}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func flowListIsUnfiltered(input flowlist.Input) bool {
	return input.Name == "" && input.OwnerName == "" && input.ProjectLUID == "" && input.ProjectName == ""
}

func (c *remoteContentCommands) InspectFlow(ctx context.Context, input flowinspect.Input) (flowinspect.Output, error) {
	if err := flowinspect.ValidateInput(input); err != nil {
		return flowinspect.Output{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return flowinspect.Output{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheFlowGetResolver{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := flowinspect.New(resolver).Execute(ctx, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowinspect.Output{}, remoteSetupError("flow.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := flowinspect.New(flowGetResolver{connection.flows}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "flow", output.Flow.LUID, output.Flow.Name, output.Flow.ProjectPath, output.Flow.OwnerLUID, "detail", observedAt, output.Flow)
	if encodeErr == nil {
		writeThrough(c.cacheStore(input.Environment), []cache.ResourceEntry{entry})
	}
	return output, nil
}

func (c *remoteContentCommands) PullFlow(ctx context.Context, input flowpull.Input) (flowpull.Output, error) {
	if err := flowpull.ValidateInput(input); err != nil {
		return flowpull.Output{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return flowpull.Output{}, capabilitySetupError("flow.pull.workspace", "flow.pull", input.Environment, input.Site, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
	input.WorkspaceName = workspace.Name
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowpull.Output{}, remoteSetupError("flow.pull", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.SiteLUID = connection.siteLUID
	input.ServerOrigin, err = artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return flowpull.Output{}, remoteSetupError("flow.pull", input.Environment, input.Site, connection.environment, err)
	}

	reader := flowPullReader{flows: connection.flows, lineage: connection.lineage}
	return flowpull.New(reader, flowArtifactWriter{artifact.NewFlowManager(c.runtime.now)}).Execute(ctx, input)
}

func (c *remoteContentCommands) PublishFlow(ctx context.Context, input flowpublish.Input, preview bool) (flowpublish.Output, error) {
	if err := flowpublish.ValidateInput(input); err != nil {
		return flowpublish.Output{}, err
	}
	manager := artifact.NewFlowManager(c.runtime.now)
	var reader flowpublish.ArtifactReader
	if input.File != "" {
		if _, err := artifact.ReadNative(ctx, input.File, "flow"); err != nil {
			return flowpublish.Output{}, capabilitySetupError("flow.publish.file", "flow.publish", input.Environment, input.Site, "Native flow validation failed.", "Select a valid native flow file, then retry.", err)
		}
		input.ArtifactPath = input.File
		reader = nativeFlowArtifactReader{}
	} else {
		workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
		if err != nil {
			return flowpublish.Output{}, capabilitySetupError("flow.publish.workspace", "flow.publish", input.Environment, input.Site, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
		}
		managed, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: "flow", Path: input.ArtifactPath, LUID: input.ArtifactID, Name: input.ArtifactName})
		if err != nil {
			if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
				return flowpublish.Output{}, mapArtifactResolutionError("flow.publish", workspace.Name, input.ArtifactID, err)
			}
			return flowpublish.Output{}, capabilitySetupError("flow.publish.artifact", "flow.publish", input.Environment, input.Site, "Flow artifact resolution failed.", "Select one exact workspace-relative managed flow artifact, then retry.", err)
		}
		absolutePath := filepath.Join(workspace.Root, filepath.FromSlash(managed.Path))
		input.WorkspaceName = workspace.Name
		_, err = manager.Read(ctx, absolutePath)
		if err != nil {
			return flowpublish.Output{}, capabilitySetupError("flow.publish.artifact", "flow.publish", input.Environment, input.Site, "Flow artifact read failed.", "Repair or pull the exact flow artifact, then retry.", err)
		}
		input.ArtifactPath = absolutePath
		reader = flowArtifactReader{manager: manager, displayPath: managed.Path}
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowpublish.Output{}, remoteSetupError("flow.publish", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	var lifecycle *publication
	adapter := flowPublishAdapter{flows: connection.flows, projects: connection.projects, changes: connection.flowChanges, runtime: c.runtime, environment: input.Environment, sourcePath: input.ArtifactPath, lifecycle: &lifecycle}
	if managed, ok := reader.(flowArtifactReader); ok {
		adapter.sourcePath = managed.displayPath
	}
	out, err := flowpublish.New(reader, adapter, adapter).Execute(ctx, input, preview)
	if out.Result != nil && out.Result.Status != "" && lifecycle != nil {
		var saveErr error
		out.Result.ReceiptPath, saveErr = lifecycle.record(ctx, "", out.Result.Status, out.Result.FlowLUID, out.Result.TableauRequestID, "")
		err = errors.Join(err, saveErr)
	}
	return out, err
}

func (c *remoteContentCommands) MoveFlow(ctx context.Context, input flowmove.Input, preview bool) (flowmove.Output, error) {
	if err := flowmove.ValidateInput(input); err != nil {
		return flowmove.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowmove.Output{}, remoteSetupError("flow.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := flowMoveAdapter{flows: connection.flows, projects: connection.projects, changes: connection.flowChanges}
	return flowmove.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) DeleteFlow(ctx context.Context, input flowdelete.Input, preview bool) (flowdelete.Output, error) {
	if err := flowdelete.ValidateInput(input); err != nil {
		return flowdelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowdelete.Output{}, remoteSetupError("flow.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := flowDeleteAdapter{flows: connection.flows, changes: connection.flowChanges}
	return flowdelete.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) DeleteWorkbook(ctx context.Context, input workbookdelete.Input, preview bool) (workbookdelete.Output, error) {
	if err := workbookdelete.ValidateInput(input); err != nil {
		return workbookdelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return workbookdelete.Output{}, remoteSetupError("workbook.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := workbookDeleteAdapter{workbooks: connection.workbooks}
	return workbookdelete.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) PullLineage(ctx context.Context, input lineagepull.Input) (lineagepull.Output, error) {
	if err := lineagepull.ValidateInput(input); err != nil {
		return lineagepull.Output{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return lineagepull.Output{}, capabilitySetupError("lineage.pull.workspace", "lineage.pull", input.Environment, input.Site, "Lineage workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
	input.WorkspaceName = workspace.Name
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return lineagepull.Output{}, remoteSetupError("lineage.pull", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.SiteLUID = connection.siteLUID
	input.ServerOrigin, err = artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return lineagepull.Output{}, remoteSetupError("lineage.pull", input.Environment, input.Site, connection.environment, err)
	}

	resolver := lineageResolver{workbooks: connection.workbooks, flows: connection.flows, datasources: connection.datasources, projects: connection.projects}
	return lineagepull.New(resolver, lineageReader{connection.lineage}, lineageArtifactWriter{artifact.NewLineageManager(c.runtime.now)}).Execute(ctx, input)
}

func remoteSetupError(operation, environment, site string, resolved config.Environment, err error) error {
	environment, site = resolvedTarget(environment, site, resolved)
	return capabilitySetupError(operation+".setup", operation, environment, site, "Tableau operation setup failed.", "Review the selected environment, site, and PAT configuration.", err)
}

type projectListReader struct{ adapter *resourceproject.Adapter }

func (r projectListReader) ListProjects(ctx context.Context, input projectlist.PageRequest) (projectlist.Page, error) {
	page, err := r.adapter.ListProjects(ctx, resourceproject.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel})
	items := make([]projectlist.Project, len(page.Items))
	for index, item := range page.Items {
		items[index] = projectListItem(item)
	}
	return projectlist.Page{Number: page.Number, Size: page.Size, Total: page.Total, Projects: items, RequestID: page.RequestID}, err
}

func projectListItem(item resourceproject.Project) projectlist.Project {
	return projectlist.Project{LUID: item.LUID, Name: item.Name, ParentLUID: item.ParentLUID, Description: item.Description, OwnerLUID: item.OwnerLUID, TopLevel: item.TopLevel, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ProjectCount: item.ProjectCount, WorkbookCount: item.WorkbookCount, ViewCount: item.ViewCount, DatasourceCount: item.DatasourceCount}
}

type projectGetResolver struct{ adapter *resourceproject.Adapter }

func (r projectGetResolver) ResolveProject(ctx context.Context, selector identity.Selector) (projectinspect.Project, error) {
	item, err := r.adapter.ResolveProject(ctx, selector)
	return projectinspect.Project{LUID: item.LUID, Name: item.Name, Path: item.Path, ParentLUID: item.ParentLUID, Description: item.Description, OwnerLUID: item.OwnerLUID, TopLevel: item.TopLevel, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ProjectCount: item.ProjectCount, WorkbookCount: item.WorkbookCount, ViewCount: item.ViewCount, DatasourceCount: item.DatasourceCount, RequestID: item.RequestID}, err
}

type projectCreateAdapter struct {
	projects *resourceproject.Adapter
	changes  *resourceproject.MutationAdapter
	resolved map[string]resourceproject.Project
}

func (a projectCreateAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (projectcreate.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	if err == nil && a.resolved != nil {
		a.resolved[item.LUID] = item
	}
	return toProjectCreate(item), err
}

func (a projectCreateAdapter) FindProjectCollisions(ctx context.Context, name, parentLUID string) ([]projectcreate.Project, error) {
	items, err := a.projects.FindProjectCollisions(ctx, name, parentLUID)
	result := make([]projectcreate.Project, len(items))
	for index, item := range items {
		result[index] = toProjectCreate(item)
	}
	return result, err
}

func (a projectCreateAdapter) CreateProject(ctx context.Context, input projectcreate.CreateRequest) (projectcreate.Result, error) {
	result, err := a.changes.CreateProject(ctx, tableauproject.CreateRequest{Name: input.Name, Description: input.Description, ParentLUID: input.ParentLUID, ContentPermissions: input.ContentPermissions})
	if err != nil {
		return projectcreate.Result{}, err
	}
	item := normalizeSuccessfulProjectMutation(ctx, a.projects, a.resolved, result.Project)
	return projectcreate.Result{Status: result.Status, Project: toProjectCreate(item), TableauRequestID: result.TableauRequestID}, nil
}

func toProjectCreate(item resourceproject.Project) projectcreate.Project {
	return projectcreate.Project{LUID: item.LUID, Name: item.Name, Path: item.Path, ParentLUID: item.ParentLUID, Description: item.Description, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}
}

type projectUpdateAdapter struct {
	projects *resourceproject.Adapter
	changes  *resourceproject.MutationAdapter
	resolved map[string]resourceproject.Project
}

func (a projectUpdateAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (projectupdate.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	if err == nil && a.resolved != nil {
		a.resolved[item.LUID] = item
	}
	return toProjectUpdate(item), err
}

func (a projectUpdateAdapter) UpdateProject(ctx context.Context, input projectupdate.UpdateRequest) (projectupdate.Result, error) {
	result, err := a.changes.UpdateProject(ctx, tableauproject.UpdateRequest{LUID: input.LUID, Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions})
	if err != nil {
		return projectupdate.Result{}, err
	}
	item := normalizeSuccessfulProjectMutation(ctx, a.projects, a.resolved, result.Project)
	return projectupdate.Result{Status: result.Status, Project: toProjectUpdate(item), TableauRequestID: result.TableauRequestID}, nil
}

func toProjectUpdate(item resourceproject.Project) projectupdate.Project {
	return projectupdate.Project{LUID: item.LUID, Name: item.Name, Path: item.Path, ParentLUID: item.ParentLUID, Description: item.Description, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}
}

type projectDeleteAdapter struct {
	projects *resourceproject.Adapter
	changes  *resourceproject.MutationAdapter
}

func (a projectDeleteAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (projectdelete.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return projectdelete.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a projectDeleteAdapter) DeleteProject(ctx context.Context, luid string) (projectdelete.Result, error) {
	result, err := a.changes.DeleteProject(ctx, luid)
	return projectdelete.Result{Status: result.Status, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

type flowListReader struct{ adapter *resourceflow.Adapter }

func (r flowListReader) ListFlows(ctx context.Context, input flowlist.PageRequest) (flowlist.Page, error) {
	page, err := r.adapter.ListFlows(ctx, tableauflow.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName})
	items := make([]flowlist.Flow, len(page.Items))
	for index, item := range page.Items {
		items[index] = flowlist.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: item.ProjectPath, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...)}
	}
	return flowlist.Page{Number: page.Number, Size: page.Size, Total: page.Total, Flows: items, RequestID: page.RequestID}, err
}

type flowGetResolver struct{ adapter *resourceflow.Adapter }

func (r flowGetResolver) ResolveFlow(ctx context.Context, selector identity.Selector) (flowinspect.Flow, error) {
	item, err := r.adapter.ResolveFlow(ctx, selector)
	return toFlowGet(item), err
}

func toFlowGet(item resourceflow.Flow) flowinspect.Flow {
	parameters := make([]flowinspect.Parameter, len(item.Parameters))
	for index, parameter := range item.Parameters {
		parameters[index] = flowinspect.Parameter{LUID: parameter.LUID, Name: parameter.Name, Type: parameter.Type, Description: parameter.Description, Value: parameter.Value, Required: parameter.Required}
	}
	steps := make([]flowinspect.OutputStep, len(item.OutputSteps))
	for index, step := range item.OutputSteps {
		steps[index] = flowinspect.OutputStep{LUID: step.LUID, Name: step.Name}
	}
	return flowinspect.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...), Parameters: parameters, OutputSteps: steps, RequestID: item.RequestID}
}

type flowPullReader struct {
	flows   *resourceflow.Adapter
	lineage *resourcelineage.Adapter
}

func (r flowPullReader) ResolveFlow(ctx context.Context, selector identity.Selector) (flowpull.Flow, error) {
	item, err := r.flows.ResolveFlow(ctx, selector)
	return flowpull.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, FileType: item.FileType}, err
}

func (r flowPullReader) DownloadFlow(ctx context.Context, luid string) (flowpull.Download, error) {
	progress.SetLabel(ctx, "Downloading flow")
	item, err := r.flows.DownloadFlow(ctx, luid)
	return flowpull.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (r flowPullReader) CaptureLineage(ctx context.Context, input flowpull.LineageRequest) (flowpull.Lineage, error) {
	progress.SetLabel(ctx, "Reading flow metadata")
	graph, err := r.lineage.Capture(ctx, resourcelineage.Request{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	return flowPullLineage(graph), err
}

func flowPullLineage(graph resourcelineage.Graph) flowpull.Lineage {
	nodes := make([]flowpull.LineageNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		nodes[index] = flowpull.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]flowpull.LineageEdge, len(graph.Edges))
	for index, edge := range graph.Edges {
		edges[index] = flowpull.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return flowpull.Lineage{Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}
}

type flowArtifactWriter struct{ manager *artifact.FlowManager }

func (w flowArtifactWriter) WriteFlow(ctx context.Context, input flowpull.Artifact) (flowpull.ArtifactResult, error) {
	progress.SetLabel(ctx, "Saving flow files")
	nodes := make([]artifact.LineageNode, len(input.Lineage.Nodes))
	for index, node := range input.Lineage.Nodes {
		nodes[index] = artifact.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]artifact.LineageEdge, len(input.Lineage.Edges))
	for index, edge := range input.Lineage.Edges {
		edges[index] = artifact.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	direction, depth := input.Lineage.Direction, input.Lineage.Depth
	if direction == "" {
		direction = "both"
	}
	if depth == 0 {
		depth = 1
	}
	result, err := w.manager.Pull(ctx, artifact.FlowPull{Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite, Metadata: artifact.FlowMetadata{Name: input.Name, TableauID: input.TableauID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID, FileType: input.FileType}, Lineage: artifact.LineageDocument{Complete: input.Lineage.Complete, Direction: direction, Depth: depth, Failure: artifactLineageFailure(input.Lineage.Failure), Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Lineage.Warnings...)}})
	if err != nil {
		return flowpull.ArtifactResult{}, err
	}
	canonicalPath, err := containWorkspacePath(input.Workspace, result.CanonicalPath, "canonical path")
	if err != nil {
		return flowpull.ArtifactResult{}, err
	}
	lineagePath, err := containWorkspacePath(input.Workspace, filepath.Join(input.Workspace, filepath.FromSlash(result.LineagePath)), "lineage path")
	if err != nil {
		return flowpull.ArtifactResult{}, err
	}
	return flowpull.ArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: canonicalPath, BaselineFingerprint: result.BaselineFingerprint, LineagePath: lineagePath, Warnings: append([]string(nil), result.Warnings...)}, nil
}

// containWorkspacePath normalizes an absolute artifact path to a
// workspace-relative slash path and rejects any path that escapes the
// resolved workspace tree.
func containWorkspacePath(workspace, absolute, label string) (string, error) {
	relative, err := filepath.Rel(workspace, absolute)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", errors.New("flow artifact " + label + " escapes the resolved workspace")
	}
	return relative, nil
}

type flowArtifactReader struct {
	manager     *artifact.FlowManager
	displayPath string
}

func (r flowArtifactReader) ReadFlow(ctx context.Context, path string) (flowpublish.Artifact, error) {
	item, err := r.manager.Read(ctx, path)
	return flowpublish.Artifact{TableauID: item.Metadata.TableauID, Path: r.displayPath, PayloadPath: item.PayloadPath, Filename: item.Filename, Name: item.Name, Fingerprint: item.Fingerprint, SourceEnvironment: item.Metadata.SourceEnvironment, SourceSite: item.Metadata.SourceSite, SourceProjectName: item.Metadata.SourceProjectName, SourceProjectID: item.Metadata.SourceProjectID, Size: item.Size}, err
}

type flowPublishAdapter struct {
	flows                   *resourceflow.Adapter
	projects                *resourceproject.Adapter
	changes                 *resourceflow.MutationAdapter
	runtime                 *runtimeDependencies
	environment, sourcePath string
	lifecycle               **publication
}

func (a flowPublishAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (flowpublish.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return flowpublish.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a flowPublishAdapter) FindFlows(ctx context.Context, name, projectLUID string) ([]flowpublish.Flow, error) {
	items, err := a.flows.FindFlows(ctx, name, projectLUID)
	result := make([]flowpublish.Flow, len(items))
	for index, item := range items {
		result[index] = flowpublish.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID}
	}
	return result, err
}

func (a flowPublishAdapter) Prepare(ctx context.Context, input flowpublish.PublishRequest) (flowpublish.PreparedPublish, error) {
	if a.runtime != nil {
		p, err := a.runtime.publication(ctx, a.environment, "flow", a.sourcePath, input.ProjectLUID, input.Name)
		if err != nil {
			return nil, err
		}
		if a.lifecycle != nil {
			*a.lifecycle = p
		}
	}
	prepared, err := a.changes.PrepareFlow(ctx, tableauflow.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Overwrite: input.Overwrite})
	if err != nil {
		return nil, err
	}
	return preparedFlowPublish{prepared}, nil
}

type preparedFlowPublish struct{ prepared tableauflow.PreparedPublish }

func (p preparedFlowPublish) Commit(ctx context.Context) (flowpublish.Result, error) {
	progress.SetLabel(ctx, "Uploading and submitting flow")
	result, err := p.prepared.Commit(ctx)
	return flowpublish.Result{Status: result.Status, FlowLUID: result.FlowLUID, FlowName: result.FlowName, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

type flowMoveAdapter struct {
	flows    *resourceflow.Adapter
	projects *resourceproject.Adapter
	changes  *resourceflow.MutationAdapter
}

func (a flowMoveAdapter) ResolveFlow(ctx context.Context, selector identity.Selector) (flowmove.Flow, error) {
	item, err := a.flows.ResolveFlow(ctx, selector)
	return flowmove.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}, err
}

func (a flowMoveAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (flowmove.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return flowmove.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a flowMoveAdapter) MoveFlow(ctx context.Context, flowLUID, projectLUID string) (flowmove.Result, error) {
	result, err := a.changes.MoveFlow(ctx, flowLUID, projectLUID)
	return flowmove.Result{Status: result.Status, FlowLUID: result.FlowLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

type flowDeleteAdapter struct {
	flows   *resourceflow.Adapter
	changes *resourceflow.MutationAdapter
}

type workbookDeleteAdapter struct{ workbooks *resourceworkbook.Adapter }

func (a workbookDeleteAdapter) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookdelete.Workbook, error) {
	item, err := a.workbooks.ResolveWorkbook(ctx, selector)
	return workbookdelete.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}, err
}

func (a workbookDeleteAdapter) DeleteWorkbook(ctx context.Context, luid string) (workbookdelete.Result, error) {
	result, err := a.workbooks.DeleteWorkbook(ctx, luid)
	return workbookdelete.Result{Status: result.Status, WorkbookLUID: result.WorkbookLUID, TableauRequestID: result.TableauRequestID}, err
}

func (a flowDeleteAdapter) ResolveFlow(ctx context.Context, selector identity.Selector) (flowdelete.Flow, error) {
	item, err := a.flows.ResolveFlow(ctx, selector)
	return flowdelete.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}, err
}

func (a flowDeleteAdapter) DeleteFlow(ctx context.Context, luid string) (flowdelete.Result, error) {
	result, err := a.changes.DeleteFlow(ctx, luid)
	return flowdelete.Result{Status: result.Status, FlowLUID: result.FlowLUID, TableauRequestID: result.TableauRequestID}, err
}

type lineageResolver struct {
	workbooks   *resourceworkbook.Adapter
	flows       *resourceflow.Adapter
	datasources *resourcedatasource.Adapter
	projects    *resourceproject.Adapter
}

func (r lineageResolver) ResolveLineageResource(ctx context.Context, kind string, selector identity.Selector) (lineagepull.Resource, error) {
	switch kind {
	case "workbook":
		item, err := r.workbooks.ResolveWorkbook(ctx, selector)
		return lineagepull.Resource{Kind: kind, LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath}, err
	case "flow":
		item, err := r.flows.ResolveFlow(ctx, selector)
		return lineagepull.Resource{Kind: kind, LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath}, err
	case "published_datasource":
		item, err := r.datasources.ResolveDatasource(ctx, selector)
		if err != nil {
			return lineagepull.Resource{}, err
		}
		return lineagepull.Resource{Kind: kind, LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath}, nil
	default:
		return lineagepull.Resource{}, errors.New("unsupported lineage resource kind")
	}
}

type lineageReader struct{ adapter *resourcelineage.Adapter }

func (r lineageReader) CaptureLineage(ctx context.Context, input lineagepull.CaptureRequest) (lineagepull.Graph, error) {
	graph, err := r.adapter.Capture(ctx, resourcelineage.Request{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	nodes := make([]lineagepull.Node, len(graph.Nodes))
	copy(nodes, graph.Nodes)
	edges := make([]lineagepull.Edge, len(graph.Edges))
	copy(edges, graph.Edges)
	return lineagepull.Graph{RootMetadataID: graph.RootMetadataID, Direction: graph.Direction, Depth: graph.Depth, Complete: graph.Complete, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...), RequestIDs: append([]string(nil), graph.RequestIDs...)}, err
}

type lineageArtifactWriter struct{ manager *artifact.LineageManager }

func (w lineageArtifactWriter) WriteLineage(ctx context.Context, input lineagepull.Artifact) (lineagepull.ArtifactResult, error) {
	nodes := make([]artifact.LineageNode, len(input.Nodes))
	for index, node := range input.Nodes {
		nodes[index] = artifact.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]artifact.LineageEdge, len(input.Edges))
	for index, edge := range input.Edges {
		edges[index] = artifact.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	result, err := w.manager.Pull(ctx, artifact.LineagePull{Workspace: input.Workspace, CountsKnown: input.CountsKnown, Overwrite: input.Overwrite, Metadata: artifact.LineageMetadata{ResourceKind: input.Resource.Kind, Name: input.Resource.Name, TableauID: input.Resource.LUID, MetadataID: input.Resource.MetadataID, ProjectPath: input.Resource.ProjectPath, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site}, Lineage: artifact.LineageDocument{Complete: input.Complete, Direction: input.Direction, Depth: input.Depth, Failure: artifactLineageFailure(input.Failure), Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Warnings...)}})
	return lineagepull.ArtifactResult{Path: result.Path, LineagePath: result.LineagePath, Fingerprint: result.Fingerprint}, err
}
