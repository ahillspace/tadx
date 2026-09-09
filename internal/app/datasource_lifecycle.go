package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	datasourcedelete "github.com/ahillspace/tadx/actions/datasource/delete"
	datasourcepublish "github.com/ahillspace/tadx/actions/datasource/publish"
	datasourcepull "github.com/ahillspace/tadx/actions/datasource/pull"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

func (c *remoteContentCommands) PullDatasource(ctx context.Context, input datasourcepull.Input) (datasourcepull.Output, error) {
	if err := datasourcepull.ValidateInput(input); err != nil {
		return datasourcepull.Output{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return datasourcepull.Output{}, capabilitySetupError("datasource.pull.workspace", "datasource.pull", input.Environment, input.Site, "Datasource workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
	input.WorkspaceName = workspace.Name
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourcepull.Output{}, remoteSetupError("datasource.pull", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.SiteLUID = connection.siteLUID
	input.ServerOrigin, err = artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return datasourcepull.Output{}, remoteSetupError("datasource.pull", input.Environment, input.Site, connection.environment, err)
	}

	reader := datasourcePullReader{datasources: connection.datasources, lineage: connection.lineage}
	return datasourcepull.New(reader, datasourceArtifactWriter{artifact.NewDatasourceManager(c.runtime.now)}).Execute(ctx, input)
}

func (c *remoteContentCommands) PublishDatasource(ctx context.Context, input datasourcepublish.Input, preview bool) (datasourcepublish.Output, error) {
	if err := datasourcepublish.ValidateInput(input); err != nil {
		return datasourcepublish.Output{}, err
	}
	manager := artifact.NewDatasourceManager(c.runtime.now)
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return datasourcepublish.Output{}, capabilitySetupError("datasource.publish.workspace", "datasource.publish", input.Environment, input.Site, "Datasource workspace resolution failed.", "Select or configure the logical workspace containing the exact managed datasource artifact, then retry.", err)
	}
	managed, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: "datasource", Path: input.ArtifactPath})
	if err != nil {
		return datasourcepublish.Output{}, capabilitySetupError("datasource.publish.artifact", "datasource.publish", input.Environment, input.Site, "Datasource artifact resolution failed.", "Select one exact workspace-relative managed datasource artifact, then retry.", err)
	}
	absolutePath := filepath.Join(workspace.Root, filepath.FromSlash(managed.Path))
	input.WorkspaceName = workspace.Name
	local, err := manager.Read(ctx, absolutePath)
	if err != nil {
		return datasourcepublish.Output{}, capabilitySetupError("datasource.publish.artifact", "datasource.publish", input.Environment, input.Site, "Datasource artifact read failed.", "Repair or pull the exact datasource artifact, then retry.", err)
	}
	if input.SourceDefaulted {
		input.Environment = local.SourceEnvironment
		input.ProjectSelector = identity.Selector{LUID: identity.LUID(local.SourceProjectID)}
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourcepublish.Output{}, remoteSetupError("datasource.publish", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site, input.ArtifactPath = connection.environment.Alias, connection.environment.SiteContentURL, absolutePath
	input.TargetResolved = true
	adapter := datasourcePublishAdapter{datasources: connection.datasources, projects: connection.projects, changes: connection.datasourceChanges}
	return datasourcepublish.New(datasourceArtifactReader{manager: manager, displayPath: managed.Path}, adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) DeleteDatasource(ctx context.Context, input datasourcedelete.Input, preview bool) (datasourcedelete.Output, error) {
	if err := datasourcedelete.ValidateInput(input); err != nil {
		return datasourcedelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourcedelete.Output{}, remoteSetupError("datasource.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := datasourceDeleteAdapter{datasources: connection.datasources, changes: connection.datasourceChanges}
	return datasourcedelete.New(adapter, adapter).Execute(ctx, input, preview)
}

type datasourcePullReader struct {
	datasources *resourcedatasource.Adapter
	lineage     *resourcelineage.Adapter
}

func (r datasourcePullReader) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourcepull.Datasource, error) {
	item, err := r.datasources.ResolveDatasource(ctx, selector)
	return datasourcepull.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}, err
}

func (r datasourcePullReader) DownloadDatasource(ctx context.Context, luid string) (datasourcepull.Download, error) {
	item, err := r.datasources.DownloadDatasource(ctx, luid)
	return datasourcepull.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (r datasourcePullReader) CaptureLineage(ctx context.Context, input datasourcepull.LineageRequest) (datasourcepull.Lineage, error) {
	graph, err := r.lineage.Capture(ctx, resourcelineage.Request{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	nodes := make([]datasourcepull.LineageNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		nodes[index] = datasourcepull.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]datasourcepull.LineageEdge, len(graph.Edges))
	for index, edge := range graph.Edges {
		edges[index] = datasourcepull.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return datasourcepull.Lineage{Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}, err
}

type datasourceArtifactWriter struct{ manager *artifact.DatasourceManager }

func (w datasourceArtifactWriter) WriteDatasource(ctx context.Context, input datasourcepull.Artifact) (datasourcepull.ArtifactResult, error) {
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
	result, err := w.manager.Pull(ctx, artifact.DatasourcePull{
		Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite,
		Metadata: artifact.DatasourceMetadata{Name: input.Name, TableauID: input.TableauID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID},
		Lineage:  artifact.LineageDocument{Complete: input.Lineage.Complete, Direction: direction, Depth: depth, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Lineage.Warnings...)},
	})
	if err != nil {
		return datasourcepull.ArtifactResult{}, err
	}
	canonicalPath, err := containDatasourceWorkspacePath(input.Workspace, result.CanonicalPath, "canonical path")
	if err != nil {
		return datasourcepull.ArtifactResult{}, err
	}
	lineagePath, err := containDatasourceWorkspacePath(input.Workspace, filepath.Join(input.Workspace, filepath.FromSlash(result.LineagePath)), "lineage path")
	if err != nil {
		return datasourcepull.ArtifactResult{}, err
	}
	// LineageStatus and CountsKnown are single-sourced from lineage completeness in the
	// pull action; leave LineageStatus unset here so the two fields cannot diverge.
	return datasourcepull.ArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: canonicalPath, BaselineFingerprint: result.BaselineFingerprint, LineagePath: lineagePath, CompositionStatus: result.CompositionStatus, ParentDataSourceURLs: append([]string(nil), result.ParentDataSourceURLs...), Warnings: append([]string(nil), result.Warnings...)}, nil
}

func containDatasourceWorkspacePath(workspace, absolute, label string) (string, error) {
	relative, err := filepath.Rel(workspace, absolute)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", errors.New("datasource artifact " + label + " escapes the resolved workspace")
	}
	return relative, nil
}

type datasourceArtifactReader struct {
	manager     *artifact.DatasourceManager
	displayPath string
}

func (r datasourceArtifactReader) ReadDatasource(ctx context.Context, path string) (datasourcepublish.Artifact, error) {
	item, err := r.manager.Read(ctx, path)
	return datasourcepublish.Artifact{Path: r.displayPath, PayloadPath: item.PayloadPath, Filename: item.Filename, Name: item.Name, TableauID: item.TableauID, Fingerprint: item.Fingerprint, SourceEnvironment: item.SourceEnvironment, SourceSite: item.SourceSite, SourceProjectName: item.SourceProjectName, SourceProjectID: item.SourceProjectID, Size: item.Size, CompositionStatus: item.CompositionStatus, ParentDataSourceURLs: append([]string(nil), item.ParentDataSourceURLs...)}, err
}

type datasourcePublishAdapter struct {
	datasources *resourcedatasource.Adapter
	projects    *resourceproject.Adapter
	changes     *resourcedatasource.MutationAdapter
}

func (a datasourcePublishAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (datasourcepublish.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return datasourcepublish.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a datasourcePublishAdapter) FindDatasources(ctx context.Context, name, projectLUID string) ([]datasourcepublish.Datasource, error) {
	items, err := a.datasources.FindDatasources(ctx, name, projectLUID)
	result := make([]datasourcepublish.Datasource, len(items))
	for index, item := range items {
		result[index] = datasourcepublish.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID}
	}
	return result, err
}

func (a datasourcePublishAdapter) ResolvePublishedDatasource(ctx context.Context, name, projectLUID string) (datasourcepublish.Datasource, error) {
	item, err := a.datasources.ResolvePublishedDatasource(ctx, name, projectLUID)
	return datasourcepublish.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID}, err
}

func (a datasourcePublishAdapter) Prepare(ctx context.Context, input datasourcepublish.PublishRequest) (datasourcepublish.PreparedPublish, error) {
	mode, err := datasourcePublishMode(input.Mode)
	if err != nil {
		return nil, err
	}
	prepared, err := a.changes.PrepareDatasource(ctx, tableaudatasource.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Mode: mode, ParentDataSourceURLs: append([]string(nil), input.ParentDataSourceURLs...), AsJob: input.AsJob})
	if err != nil {
		return nil, err
	}
	return preparedDatasourcePublish{prepared: prepared}, nil
}

func datasourcePublishMode(mode datasourcepublish.Mode) (tableaudatasource.PublishMode, error) {
	switch mode {
	case datasourcepublish.ModeCreate:
		return tableaudatasource.PublishCreate, nil
	case datasourcepublish.ModeOverwrite:
		return tableaudatasource.PublishOverwrite, nil
	case datasourcepublish.ModeAppend:
		return tableaudatasource.PublishAppend, nil
	case datasourcepublish.ModeReplace:
		return tableaudatasource.PublishReplace, nil
	default:
		return "", errors.New("unsupported datasource publish mode")
	}
}

type preparedDatasourcePublish struct {
	prepared tableaudatasource.PreparedPublish
}

func (p preparedDatasourcePublish) Commit(ctx context.Context) (datasourcepublish.Result, error) {
	result, err := p.prepared.Commit(ctx)
	return datasourcepublish.Result{Status: result.Status, DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, ProjectLUID: result.ProjectLUID, JobID: result.JobID, TableauRequestID: result.TableauRequestID}, err
}

type datasourceDeleteAdapter struct {
	datasources *resourcedatasource.Adapter
	changes     *resourcedatasource.MutationAdapter
}

func (a datasourceDeleteAdapter) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourcedelete.Datasource, error) {
	item, err := a.datasources.ResolveDatasource(ctx, selector)
	return datasourcedelete.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}, err
}

func (a datasourceDeleteAdapter) DeleteDatasource(ctx context.Context, luid string) (datasourcedelete.Result, error) {
	result, err := a.changes.DeleteDatasource(ctx, luid)
	return datasourcedelete.Result{Status: result.Status, DatasourceLUID: result.DatasourceLUID, TableauRequestID: result.TableauRequestID}, err
}
