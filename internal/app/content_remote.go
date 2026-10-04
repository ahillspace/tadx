package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	lineagepull "github.com/ahillspace/tadx/actions/lineage"
	projectops "github.com/ahillspace/tadx/actions/project"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
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
)

type remoteContentCommands struct{ runtime *runtimeDependencies }

func newRemoteContentCommands(runtime *runtimeDependencies) *remoteContentCommands {
	return &remoteContentCommands{runtime: runtime}
}

func (c *remoteContentCommands) dependencies() *contentcli.Dependencies {
	projects := projectops.New(projectops.Ports{Provider: projectProvider{commands: c}})
	mutations := contentMutationProvider{commands: c}
	workbooks := workbookops.New(workbookops.Ports{Mutation: mutations, Read: workbookReadProvider{commands: c}})
	datasources := datasourceops.New(datasourceops.Ports{Mutation: mutations, Read: &datasourceReadProvider{commands: c}, Schema: datasourceSchemaProvider{commands: c}})
	flows := flowops.New(flowops.Ports{Mutation: mutations, Read: flowReadProvider{commands: c}})
	return &contentcli.Dependencies{
		WorkbookLister: workbooks, WorkbookInspector: workbooks, WorkbookDeleter: workbooks, WorkbookMover: workbooks, WorkbookUpdater: workbooks,
		DatasourceLister: datasources, DatasourceInspector: datasources, DatasourceSchema: datasources, DatasourcePuller: c, DatasourcePublisher: c, DatasourceDeleter: datasources, DatasourceMover: datasources, DatasourceUpdater: datasources,
		ProjectLister: projects, ProjectInspector: projects, ProjectCreator: projects, ProjectUpdater: projects, ProjectDeleter: projects, ProjectMover: projects,
		FlowLister: flows, FlowInspector: flows, FlowPuller: c, FlowPublisher: c, FlowMover: flows, FlowDeleter: flows, FlowUpdater: flows,
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
				return flowops.PublishOutput{}, artifact.MapResolutionError("flow.publish", workspace.Name, input.ArtifactID, err)
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
		out.Result.ReceiptPath, saveErr = lifecycle.Record(ctx, "", out.Result.Status, out.Result.FlowLUID, out.Result.TableauRequestID, "")
		err = errors.Join(err, saveErr)
	}
	return out, err
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
