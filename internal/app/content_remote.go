package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	flowdelete "github.com/ahillspace/tadx/actions/flow/delete"
	flowget "github.com/ahillspace/tadx/actions/flow/get"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	flowmove "github.com/ahillspace/tadx/actions/flow/move"
	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	flowpull "github.com/ahillspace/tadx/actions/flow/pull"
	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	projectget "github.com/ahillspace/tadx/actions/project/get"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	"github.com/ahillspace/tadx/internal/artifact"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableaumetadata "github.com/ahillspace/tadx/internal/tableau/metadata"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type remoteContentCommands struct{ runtime *runtimeDependencies }

func newRemoteContentCommands(runtime *runtimeDependencies) *remoteContentCommands {
	return &remoteContentCommands{runtime: runtime}
}

func (c *remoteContentCommands) dependencies() *contentcli.Dependencies {
	return &contentcli.Dependencies{
		ProjectLister: c, ProjectGetter: c,
		FlowLister: c, FlowGetter: c, FlowPuller: c, FlowPublisher: c, FlowMover: c, FlowDeleter: c,
		LineagePuller: c,
	}
}

type remoteConnection struct {
	environment config.Environment
	siteLUID    string
	projects    *resourceproject.Adapter
	flows       *resourceflow.Adapter
	flowChanges *resourceflow.MutationAdapter
	lineage     *resourcelineage.Adapter
	workbooks   *resourceworkbook.Adapter
	datasources *resourcedatasource.Adapter
}

func (c *remoteContentCommands) connect(ctx context.Context, alias string, explicit bool) (remoteConnection, error) {
	connection, err := c.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return remoteConnection{environment: connection.environment}, err
	}
	projectClient := tableauproject.NewClient(connection.transport, connection.session, connection.environment.URL)
	projects := resourceproject.NewAdapter(projectClient)
	flowClient := tableauflow.NewClient(connection.transport, connection.session, connection.environment.URL)
	datasourceClient := tableaudatasource.NewClient(connection.transport, connection.session, connection.environment.URL)
	return remoteConnection{
		environment: connection.environment,
		siteLUID:    connection.session.SiteLUID(),
		projects:    projects,
		flows:       resourceflow.NewAdapter(flowClient, projects),
		flowChanges: resourceflow.NewMutationAdapter(flowClient),
		lineage:     resourcelineage.NewAdapter(tableaumetadata.NewClient(connection.transport, connection.session, connection.environment.URL)),
		workbooks:   resourceworkbook.NewAdapter(tableauworkbook.NewClient(connection.transport, connection.session, connection.environment.URL)),
		datasources: resourcedatasource.NewAdapterWithProjectResolver(datasourceClient, projects),
	}, nil
}

func (c *remoteContentCommands) ListProjects(ctx context.Context, input projectlist.Input) (projectlist.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return projectlist.Output{}, remoteSetupError("project.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return projectlist.New(projectListReader{connection.projects}).Execute(ctx, input)
}

func (c *remoteContentCommands) GetProject(ctx context.Context, input projectget.Input) (projectget.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return projectget.Output{}, remoteSetupError("project.get", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return projectget.New(projectGetResolver{connection.projects}).Execute(ctx, input)
}

func (c *remoteContentCommands) ListFlows(ctx context.Context, input flowlist.Input) (flowlist.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowlist.Output{}, remoteSetupError("flow.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return flowlist.New(flowListReader{connection.flows}).Execute(ctx, input)
}

func (c *remoteContentCommands) GetFlow(ctx context.Context, input flowget.Input) (flowget.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowget.Output{}, remoteSetupError("flow.get", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return flowget.New(flowGetResolver{connection.flows}).Execute(ctx, input)
}

func (c *remoteContentCommands) PullFlow(ctx context.Context, input flowpull.Input) (flowpull.Output, error) {
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
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return flowpull.Output{}, capabilitySetupError("flow.pull.workspace", "flow.pull", input.Environment, input.Site, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
	reader := flowPullReader{flows: connection.flows, lineage: connection.lineage}
	return flowpull.New(reader, flowArtifactWriter{artifact.NewFlowManager(c.runtime.now)}).Execute(ctx, input)
}

func (c *remoteContentCommands) PublishFlow(ctx context.Context, input flowpublish.Input, apply bool) (flowpublish.Output, error) {
	manager := artifact.NewFlowManager(c.runtime.now)
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return flowpublish.Output{}, capabilitySetupError("flow.publish.workspace", "flow.publish", input.Environment, input.Site, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	managed, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: "flow", Path: input.ArtifactPath})
	if err != nil {
		return flowpublish.Output{}, capabilitySetupError("flow.publish.artifact", "flow.publish", input.Environment, input.Site, "Flow artifact resolution failed.", "Select one exact workspace-relative managed flow artifact, then retry.", err)
	}
	absolutePath := filepath.Join(workspace.Root, filepath.FromSlash(managed.Path))
	local, err := manager.Read(ctx, absolutePath)
	if err != nil {
		return flowpublish.Output{}, capabilitySetupError("flow.publish.artifact", "flow.publish", input.Environment, input.Site, "Flow artifact read failed.", "Repair or pull the exact flow artifact, then retry.", err)
	}
	if input.Environment == "" {
		input.Environment = local.Metadata.SourceEnvironment
		input.SetProjectSelector(local.Metadata.SourceProjectID, "")
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowpublish.Output{}, remoteSetupError("flow.publish", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site, input.ArtifactPath = connection.environment.Alias, connection.environment.SiteContentURL, absolutePath
	adapter := flowPublishAdapter{flows: connection.flows, projects: connection.projects, changes: connection.flowChanges}
	return flowpublish.New(flowArtifactReader{manager: manager, displayPath: managed.Path}, adapter, adapter).Execute(ctx, input, apply)
}

func (c *remoteContentCommands) MoveFlow(ctx context.Context, input flowmove.Input, apply bool) (flowmove.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowmove.Output{}, remoteSetupError("flow.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := flowMoveAdapter{flows: connection.flows, projects: connection.projects, changes: connection.flowChanges}
	return flowmove.New(adapter, adapter).Execute(ctx, input, apply)
}

func (c *remoteContentCommands) DeleteFlow(ctx context.Context, input flowdelete.Input, apply bool) (flowdelete.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowdelete.Output{}, remoteSetupError("flow.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := flowDeleteAdapter{flows: connection.flows, changes: connection.flowChanges}
	return flowdelete.New(adapter, adapter).Execute(ctx, input, apply)
}

func (c *remoteContentCommands) PullLineage(ctx context.Context, input lineagepull.Input) (lineagepull.Output, error) {
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
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return lineagepull.Output{}, capabilitySetupError("lineage.pull.workspace", "lineage.pull", input.Environment, input.Site, "Lineage workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
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

func (r projectGetResolver) ResolveProject(ctx context.Context, selector identity.Selector) (projectget.Project, error) {
	item, err := r.adapter.ResolveProject(ctx, selector)
	return projectget.Project{LUID: item.LUID, Name: item.Name, Path: item.Path, ParentLUID: item.ParentLUID, Description: item.Description, OwnerLUID: item.OwnerLUID, TopLevel: item.TopLevel, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ProjectCount: item.ProjectCount, WorkbookCount: item.WorkbookCount, ViewCount: item.ViewCount, DatasourceCount: item.DatasourceCount, RequestID: item.RequestID}, err
}

type flowListReader struct{ adapter *resourceflow.Adapter }

func (r flowListReader) ListFlows(ctx context.Context, input flowlist.PageRequest) (flowlist.Page, error) {
	page, err := r.adapter.ListFlows(ctx, tableauflow.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName})
	items := make([]flowlist.Flow, len(page.Items))
	for index, item := range page.Items {
		items[index] = flowlist.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...)}
	}
	return flowlist.Page{Number: page.Number, Size: page.Size, Total: page.Total, Flows: items, RequestID: page.RequestID}, err
}

type flowGetResolver struct{ adapter *resourceflow.Adapter }

func (r flowGetResolver) ResolveFlow(ctx context.Context, selector identity.Selector) (flowget.Flow, error) {
	item, err := r.adapter.ResolveFlow(ctx, selector)
	return toFlowGet(item), err
}

func toFlowGet(item resourceflow.Flow) flowget.Flow {
	parameters := make([]flowget.Parameter, len(item.Parameters))
	for index, parameter := range item.Parameters {
		parameters[index] = flowget.Parameter{LUID: parameter.LUID, Name: parameter.Name, Type: parameter.Type, Description: parameter.Description, Value: parameter.Value, Required: parameter.Required}
	}
	steps := make([]flowget.OutputStep, len(item.OutputSteps))
	for index, step := range item.OutputSteps {
		steps[index] = flowget.OutputStep{LUID: step.LUID, Name: step.Name}
	}
	return flowget.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...), Parameters: parameters, OutputSteps: steps, RequestID: item.RequestID}
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
	item, err := r.flows.DownloadFlow(ctx, luid)
	return flowpull.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (r flowPullReader) CaptureLineage(ctx context.Context, input flowpull.LineageRequest) (flowpull.Lineage, error) {
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
	return flowpull.Lineage{Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}
}

type flowArtifactWriter struct{ manager *artifact.FlowManager }

func (w flowArtifactWriter) WriteFlow(ctx context.Context, input flowpull.Artifact) (flowpull.ArtifactResult, error) {
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
	result, err := w.manager.Pull(ctx, artifact.FlowPull{Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite, Metadata: artifact.FlowMetadata{Name: input.Name, TableauID: input.TableauID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID, FileType: input.FileType}, Lineage: artifact.LineageDocument{Complete: input.Lineage.Complete, Direction: direction, Depth: depth, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Lineage.Warnings...)}})
	if err != nil {
		return flowpull.ArtifactResult{}, err
	}
	canonicalPath, err := filepath.Rel(input.Workspace, result.CanonicalPath)
	if err != nil {
		return flowpull.ArtifactResult{}, err
	}
	canonicalPath = filepath.ToSlash(filepath.Clean(canonicalPath))
	if canonicalPath == ".." || strings.HasPrefix(canonicalPath, "../") {
		return flowpull.ArtifactResult{}, errors.New("flow artifact canonical path escapes the resolved workspace")
	}
	return flowpull.ArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: canonicalPath, BaselineFingerprint: result.BaselineFingerprint, LineagePath: result.LineagePath, Warnings: append([]string(nil), result.Warnings...)}, nil
}

type flowArtifactReader struct {
	manager     *artifact.FlowManager
	displayPath string
}

func (r flowArtifactReader) ReadFlow(ctx context.Context, path string) (flowpublish.Artifact, error) {
	item, err := r.manager.Read(ctx, path)
	return flowpublish.Artifact{Path: r.displayPath, PayloadPath: item.PayloadPath, Filename: item.Filename, Name: item.Name, Fingerprint: item.Fingerprint, SourceEnvironment: item.Metadata.SourceEnvironment, SourceSite: item.Metadata.SourceSite, SourceProjectName: item.Metadata.SourceProjectName, SourceProjectID: item.Metadata.SourceProjectID, Size: item.Size}, err
}

type flowPublishAdapter struct {
	flows    *resourceflow.Adapter
	projects *resourceproject.Adapter
	changes  *resourceflow.MutationAdapter
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
	prepared, err := a.changes.PrepareFlow(ctx, tableauflow.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Overwrite: input.Overwrite})
	if err != nil {
		return nil, err
	}
	return preparedFlowPublish{prepared}, nil
}

type preparedFlowPublish struct{ prepared tableauflow.PreparedPublish }

func (p preparedFlowPublish) Commit(ctx context.Context) (flowpublish.Result, error) {
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
	for index, node := range graph.Nodes {
		nodes[index] = lineagepull.Node{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]lineagepull.Edge, len(graph.Edges))
	for index, edge := range graph.Edges {
		edges[index] = lineagepull.Edge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return lineagepull.Graph{RootMetadataID: graph.RootMetadataID, Complete: graph.Complete, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...), RequestIDs: append([]string(nil), graph.RequestIDs...)}, err
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
	result, err := w.manager.Pull(ctx, artifact.LineagePull{Workspace: input.Workspace, CountsKnown: input.CountsKnown, Overwrite: input.Overwrite, Metadata: artifact.LineageMetadata{ResourceKind: input.Resource.Kind, Name: input.Resource.Name, TableauID: input.Resource.LUID, MetadataID: input.Resource.MetadataID, ProjectPath: input.Resource.ProjectPath, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site}, Lineage: artifact.LineageDocument{Complete: input.Complete, Direction: input.Direction, Depth: input.Depth, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Warnings...)}})
	return lineagepull.ArtifactResult{Path: result.Path, LineagePath: result.LineagePath, Fingerprint: result.Fingerprint}, err
}
