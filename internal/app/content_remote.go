package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	projectops "github.com/ahillspace/tadx/actions/project"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cache"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/identity"
	inventorycore "github.com/ahillspace/tadx/internal/inventory"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

type remoteContentCommands struct{ runtime *runtimeDependencies }

func newRemoteContentCommands(runtime *runtimeDependencies) *remoteContentCommands {
	return &remoteContentCommands{runtime: runtime}
}

func (c *remoteContentCommands) dependencies() *contentcli.Dependencies {
	projects := projectops.New(projectops.Ports{Provider: projectProvider{commands: c}})
	mutations := contentMutationProvider{commands: c}
	workbooks := workbookops.New(mutations)
	datasources := datasourceops.New(mutations)
	flows := flowops.New(mutations)
	return &contentcli.Dependencies{
		WorkbookLister: c, WorkbookInspector: c, WorkbookDeleter: workbooks, WorkbookMover: workbooks, WorkbookUpdater: workbooks,
		DatasourceLister: c, DatasourceInspector: c, DatasourceSchema: c, DatasourcePuller: c, DatasourcePublisher: c, DatasourceDeleter: datasources, DatasourceMover: datasources, DatasourceUpdater: datasources,
		ProjectLister: projects, ProjectInspector: projects, ProjectCreator: projects, ProjectUpdater: projects, ProjectDeleter: projects, ProjectMover: projects,
		FlowLister: c, FlowInspector: c, FlowPuller: c, FlowPublisher: c, FlowMover: flows, FlowDeleter: flows, FlowUpdater: flows,
		LineagePuller: c,
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
		inventory:               cacheTableauExecutor{transport: connection.transport, session: connection.session, serverURL: connection.environment.URL, siteLUID: connection.session.SiteLUID()},
	}, nil
}

func (c *remoteContentCommands) ListFlows(ctx context.Context, input flowops.ListInput) (result flowops.ListOutput, resultErr error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return flowops.ListOutput{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := flowops.ValidateListInput(input); err != nil {
		return flowops.ListOutput{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = inventorycore.ValidateAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return flowops.ListOutput{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheFlowListReader{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := flowops.List(ctx, reader, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauflow.ListFilter(tableauflow.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName})
	if err != nil {
		return flowops.ListOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowops.ListOutput{}, remoteSetupError("flow.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := inventorycore.Collect(ctx, connection.inventory, c.cacheStore(input.Environment), tableaucache.ScopeFlows, input.Environment, input.Site, observedAt, inventorycore.Options{MaxConcurrency: connection.environment.CacheMaxConcurrency, Filter: filter})
		if err != nil {
			return flowops.ListOutput{}, inventorycore.RefreshError("flow.list", input.Environment, input.Site, err)
		}
		reader := inventoryMemoryReader{entries: inventory.Entries, requestID: inventory.FinalRequestID()}
		reader.allowContinuation = true
		output, err := flowops.List(ctx, reader, input)
		if err != nil {
			return output, err
		}
		source, help := inventory.SourceAndHelp(observedAt, c.runtime.now)
		output.Source = source
		if help != "" {
			output.Help = append(output.Help, help)
		}
		output.RequestID = inventory.FinalRequestID()
		return output, nil
	}
	output, err := flowops.List(ctx, flowListReader{connection.flows}, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func flowListIsUnfiltered(input flowops.ListInput) bool {
	return input.Name == "" && input.OwnerName == "" && input.ProjectLUID == "" && input.ProjectName == ""
}

func (c *remoteContentCommands) InspectFlow(ctx context.Context, input flowops.InspectInput) (flowops.InspectOutput, error) {
	if err := flowops.ValidateInspectInput(input); err != nil {
		return flowops.InspectOutput{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return flowops.InspectOutput{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheFlowGetResolver{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := flowops.Inspect(ctx, resolver, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowops.InspectOutput{}, remoteSetupError("flow.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := flowops.Inspect(ctx, connection.flows, input)
	if err != nil {
		return output, err
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "flow", output.Flow.LUID, output.Flow.Name, output.Flow.ProjectPath, output.Flow.OwnerLUID, "detail", observedAt, output.CacheFlow())
	if encodeErr == nil {
		writeThrough(c.cacheStore(input.Environment), []cache.ResourceEntry{entry})
	}
	return output, nil
}

func (c *remoteContentCommands) PullFlow(ctx context.Context, input flowops.PullInput) (flowops.PullOutput, error) {
	if err := flowops.ValidatePullInput(input); err != nil {
		return flowops.PullOutput{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return flowops.PullOutput{}, capabilitySetupError("flow.pull.workspace", "flow.pull", input.Environment, input.Site, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
	input.WorkspaceName = workspace.Name
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return flowops.PullOutput{}, remoteSetupError("flow.pull", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.SiteLUID = connection.siteLUID
	input.ServerOrigin, err = artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return flowops.PullOutput{}, remoteSetupError("flow.pull", input.Environment, input.Site, connection.environment, err)
	}

	reader := flowPullReader{Adapter: connection.flows, lineage: connection.lineage}
	return flowops.Pull(ctx, reader, flowArtifactWriter{artifact.NewFlowManager(c.runtime.now)}, input)
}

func (c *remoteContentCommands) PublishFlow(ctx context.Context, input flowops.PublishInput, preview bool) (flowops.PublishOutput, error) {
	if err := flowops.ValidatePublishInput(input); err != nil {
		return flowops.PublishOutput{}, err
	}
	manager := artifact.NewFlowManager(c.runtime.now)
	var reader flowops.ArtifactReader
	if input.File != "" {
		if _, err := artifact.ReadNative(ctx, input.File, "flow"); err != nil {
			return flowops.PublishOutput{}, capabilitySetupError("flow.publish.file", "flow.publish", input.Environment, input.Site, "Native flow validation failed.", "Select a valid native flow file, then retry.", err)
		}
		input.ArtifactPath = input.File
		reader = nativeFlowArtifactReader{}
	} else {
		workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
		if err != nil {
			return flowops.PublishOutput{}, capabilitySetupError("flow.publish.workspace", "flow.publish", input.Environment, input.Site, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
		}
		managed, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: "flow", Path: input.ArtifactPath, LUID: input.ArtifactID, Name: input.ArtifactName})
		if err != nil {
			if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
				return flowops.PublishOutput{}, mapArtifactResolutionError("flow.publish", workspace.Name, input.ArtifactID, err)
			}
			return flowops.PublishOutput{}, capabilitySetupError("flow.publish.artifact", "flow.publish", input.Environment, input.Site, "Flow artifact resolution failed.", "Select one exact workspace-relative managed flow artifact, then retry.", err)
		}
		absolutePath := filepath.Join(workspace.Root, filepath.FromSlash(managed.Path))
		input.WorkspaceName = workspace.Name
		_, err = manager.Read(ctx, absolutePath)
		if err != nil {
			return flowops.PublishOutput{}, capabilitySetupError("flow.publish.artifact", "flow.publish", input.Environment, input.Site, "Flow artifact read failed.", "Repair or pull the exact flow artifact, then retry.", err)
		}
		input.ArtifactPath = absolutePath
		reader = flowArtifactReader{manager: manager, displayPath: managed.Path}
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowops.PublishOutput{}, remoteSetupError("flow.publish", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	var lifecycle *publication
	adapter := flowPublishAdapter{Adapter: connection.flows, projects: connection.projects, changes: connection.flowChanges, runtime: c.runtime, environment: input.Environment, sourcePath: input.ArtifactPath, lifecycle: &lifecycle}
	if managed, ok := reader.(flowArtifactReader); ok {
		adapter.sourcePath = managed.displayPath
	}
	out, err := flowops.NewPublish(reader, adapter, adapter).Execute(ctx, input, preview)
	if out.Result != nil && out.Result.Status != "" && lifecycle != nil {
		var saveErr error
		out.Result.ReceiptPath, saveErr = lifecycle.record(ctx, "", out.Result.Status, out.Result.FlowLUID, out.Result.TableauRequestID, "")
		err = errors.Join(err, saveErr)
	}
	return out, err
}

func (c *remoteContentCommands) PullLineage(ctx context.Context, input lineagepull.Input) (lineagepull.Output, error) {
	input, err := lineagepull.NormalizeInput(input)
	if err != nil {
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
	return lineagepull.Execute(ctx, resolver, lineageReader{connection.lineage}, lineageArtifactWriter{artifact.NewLineageManager(c.runtime.now)}, input)
}

func remoteSetupError(operation, environment, site string, resolved config.Environment, err error) error {
	environment, site = resolvedTarget(environment, site, resolved)
	return capabilitySetupError(operation+".setup", operation, environment, site, "Tableau operation setup failed.", "Review the selected environment, site, and PAT configuration.", err)
}

type flowListReader struct{ adapter *resourceflow.Adapter }

func (r flowListReader) ListFlows(ctx context.Context, input flowops.ListPageRequest) (flowops.ListPage, error) {
	page, err := r.adapter.ListFlows(ctx, tableauflow.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName})
	items := make([]flowops.Record, len(page.Items))
	for index, item := range page.Items {
		items[index] = flowops.Record{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: item.ProjectPath, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...)}
	}
	return flowops.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Flows: items, RequestID: page.RequestID}, err
}

type flowPullReader struct {
	*resourceflow.Adapter
	lineage *resourcelineage.Adapter
}

func (r flowPullReader) DownloadFlow(ctx context.Context, luid string) (flowops.PullDownload, error) {
	progress.SetLabel(ctx, "Downloading flow")
	item, err := r.Adapter.DownloadFlow(ctx, luid)
	return flowops.PullDownload{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (r flowPullReader) CaptureLineage(ctx context.Context, input flowops.PullLineageRequest) (flowops.PullLineage, error) {
	progress.SetLabel(ctx, "Reading flow metadata")
	graph, err := r.lineage.Capture(ctx, resourcelineage.Request{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	return flowPullLineage(graph), err
}

func flowPullLineage(graph resourcelineage.Graph) flowops.PullLineage {
	nodes := make([]flowops.PullLineageNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		nodes[index] = flowops.PullLineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]flowops.PullLineageEdge, len(graph.Edges))
	for index, edge := range graph.Edges {
		edges[index] = flowops.PullLineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return flowops.PullLineage{Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}
}

type flowArtifactWriter struct{ manager *artifact.FlowManager }

func (w flowArtifactWriter) WriteFlow(ctx context.Context, input flowops.PullArtifact) (flowops.PullArtifactResult, error) {
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
		return flowops.PullArtifactResult{}, err
	}
	canonicalPath, err := containWorkspacePath(input.Workspace, result.CanonicalPath, "canonical path")
	if err != nil {
		return flowops.PullArtifactResult{}, err
	}
	lineagePath, err := containWorkspacePath(input.Workspace, filepath.Join(input.Workspace, filepath.FromSlash(result.LineagePath)), "lineage path")
	if err != nil {
		return flowops.PullArtifactResult{}, err
	}
	return flowops.PullArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: canonicalPath, BaselineFingerprint: result.BaselineFingerprint, LineagePath: lineagePath, Warnings: append([]string(nil), result.Warnings...)}, nil
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

func (r flowArtifactReader) ReadFlow(ctx context.Context, path string) (flowops.PublishArtifact, error) {
	item, err := r.manager.Read(ctx, path)
	return flowops.PublishArtifact{TableauID: item.Metadata.TableauID, Path: r.displayPath, PayloadPath: item.PayloadPath, Filename: item.Filename, Name: item.Name, Fingerprint: item.Fingerprint, SourceEnvironment: item.Metadata.SourceEnvironment, SourceSite: item.Metadata.SourceSite, SourceProjectName: item.Metadata.SourceProjectName, SourceProjectID: item.Metadata.SourceProjectID, Size: item.Size}, err
}

type flowPublishAdapter struct {
	*resourceflow.Adapter
	projects                *resourceproject.Adapter
	changes                 *resourceflow.MutationAdapter
	runtime                 *runtimeDependencies
	environment, sourcePath string
	lifecycle               **publication
}

func (a flowPublishAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (flowops.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return flowops.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a flowPublishAdapter) Prepare(ctx context.Context, input flowops.PublishRequest) (flowops.PreparedPublish, error) {
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

func (p preparedFlowPublish) Commit(ctx context.Context) (flowops.PublishResult, error) {
	progress.SetLabel(ctx, "Uploading and submitting flow")
	result, err := p.prepared.Commit(ctx)
	return flowops.PublishResult{Status: result.Status, FlowLUID: result.FlowLUID, FlowName: result.FlowName, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
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
	return lineagepull.Graph{RootMetadataID: graph.RootMetadataID, Direction: graph.Direction, Depth: graph.Depth, Complete: graph.Complete, Failure: graph.Failure, Nodes: graph.Nodes, Edges: graph.Edges, Warnings: graph.Warnings, RequestIDs: graph.RequestIDs}, err
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
