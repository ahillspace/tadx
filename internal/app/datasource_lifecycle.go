package app

import (
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

func (c *remoteContentCommands) PullDatasource(ctx context.Context, input datasourceops.PullInput) (datasourceops.PullOutput, error) {
	if err := datasourceops.ValidatePullInput(input); err != nil {
		return datasourceops.PullOutput{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return datasourceops.PullOutput{}, capabilitySetupError("datasource.pull.workspace", "datasource.pull", input.Environment, input.Site, "Datasource workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
	input.WorkspaceName = workspace.Name
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourceops.PullOutput{}, remoteSetupError("datasource.pull", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.SiteLUID = connection.siteLUID
	input.ServerOrigin, err = artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return datasourceops.PullOutput{}, remoteSetupError("datasource.pull", input.Environment, input.Site, connection.environment, err)
	}

	reader := datasourcePullReader{datasources: connection.datasources, lineage: connection.lineage}
	return datasourceops.Pull(ctx, reader, datasourceArtifactWriter{artifact.NewDatasourceManager(c.runtime.now)}, input)
}

func (c *remoteContentCommands) PublishDatasource(ctx context.Context, input datasourceops.PublishInput, preview bool) (datasourceops.PublishOutput, error) {
	if err := datasourceops.ValidatePublishInput(input); err != nil {
		return datasourceops.PublishOutput{}, err
	}
	manager := artifact.NewDatasourceManager(c.runtime.now)
	var reader datasourceops.ArtifactReader
	if input.File != "" {
		if _, err := artifact.ReadNative(ctx, input.File, "datasource"); err != nil {
			return datasourceops.PublishOutput{}, capabilitySetupError("datasource.publish.file", "datasource.publish", input.Environment, input.Site, "Native datasource validation failed.", "Select a valid native datasource file, then retry.", err)
		}
		input.ArtifactPath = input.File
		reader = nativeDatasourceArtifactReader{}
	} else {
		workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
		if err != nil {
			return datasourceops.PublishOutput{}, capabilitySetupError("datasource.publish.workspace", "datasource.publish", input.Environment, input.Site, "Datasource workspace resolution failed.", "Select or configure the logical workspace containing the exact managed datasource artifact, then retry.", err)
		}
		managed, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: "datasource", Path: input.ArtifactPath, LUID: input.ArtifactID, Name: input.ArtifactName})
		if err != nil {
			if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
				return datasourceops.PublishOutput{}, mapArtifactResolutionError("datasource.publish", workspace.Name, input.ArtifactID, err)
			}
			return datasourceops.PublishOutput{}, capabilitySetupError("datasource.publish.artifact", "datasource.publish", input.Environment, input.Site, "Datasource artifact resolution failed.", "Select one exact workspace-relative managed datasource artifact, then retry.", err)
		}
		absolutePath := filepath.Join(workspace.Root, filepath.FromSlash(managed.Path))
		input.WorkspaceName = workspace.Name
		_, err = manager.Read(ctx, absolutePath)
		if err != nil {
			return datasourceops.PublishOutput{}, capabilitySetupError("datasource.publish.artifact", "datasource.publish", input.Environment, input.Site, "Datasource artifact read failed.", "Repair or pull the exact datasource artifact, then retry.", err)
		}
		input.ArtifactPath = absolutePath
		reader = datasourceArtifactReader{manager: manager, displayPath: managed.Path}
	}
	input.SourceDefaulted = false
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourceops.PublishOutput{}, remoteSetupError("datasource.publish", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	var lifecycle *publication
	adapter := datasourcePublishAdapter{datasources: connection.datasources, projects: connection.projects, changes: connection.datasourceChanges, runtime: c.runtime, environment: input.Environment, sourcePath: input.ArtifactPath, lifecycle: &lifecycle}
	if managed, ok := reader.(datasourceArtifactReader); ok {
		adapter.sourcePath = managed.displayPath
	}
	action := datasourceops.NewPublish(reader, adapter, adapter)
	out, err := action.Execute(ctx, input, preview)
	if out.Result != nil && out.Result.Status != "" && lifecycle != nil {
		var saveErr error
		out.Result.ReceiptPath, saveErr = lifecycle.record(ctx, out.Result.JobID, out.Result.Status, out.Result.DatasourceLUID, out.Result.TableauRequestID, out.Result.Verification)
		err = errors.Join(err, saveErr)
	}
	if err == nil && out.Result != nil && out.Result.Status == "pending" && lifecycle != nil && !c.runtime.publicationNoWait() {
		contentbatch.DeferCompletion(ctx, func(ctx context.Context) (any, error) { return lifecycle.completeDatasource(ctx, action, out) })
	}
	return out, err
}

func (c *remoteContentCommands) DeleteDatasource(ctx context.Context, input datasourceops.DeleteInput, preview bool) (datasourceops.DeleteOutput, error) {
	if err := datasourceops.ValidateDeleteInput(input); err != nil {
		return datasourceops.DeleteOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourceops.DeleteOutput{}, remoteSetupError("datasource.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := datasourceMutationAdapter{Adapter: connection.datasources, changes: connection.datasourceChanges}
	return datasourceops.Delete(ctx, adapter, adapter, input, preview)
}

type datasourcePullReader struct {
	datasources *resourcedatasource.Adapter
	lineage     *resourcelineage.Adapter
}

func (r datasourcePullReader) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourceops.Record, error) {
	return r.datasources.ResolveDatasource(ctx, selector)
}

func (r datasourcePullReader) DownloadDatasource(ctx context.Context, luid string) (datasourceops.Download, error) {
	progress.SetLabel(ctx, "Downloading datasource")
	item, err := r.datasources.DownloadDatasource(ctx, luid)
	return datasourceops.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (r datasourcePullReader) CaptureLineage(ctx context.Context, input datasourceops.LineageRequest) (datasourceops.Lineage, error) {
	progress.SetLabel(ctx, "Reading datasource metadata")
	graph, err := r.lineage.Capture(ctx, resourcelineage.Request{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	nodes := make([]datasourceops.LineageNode, len(graph.Nodes))
	copy(nodes, graph.Nodes)
	edges := make([]datasourceops.LineageEdge, len(graph.Edges))
	copy(edges, graph.Edges)
	return datasourceops.Lineage{Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}, err
}

type datasourceArtifactWriter struct{ manager *artifact.DatasourceManager }

func (w datasourceArtifactWriter) WriteDatasource(ctx context.Context, input datasourceops.PullArtifact) (datasourceops.PullArtifactResult, error) {
	progress.SetLabel(ctx, "Saving datasource files")
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
		Lineage:  artifact.LineageDocument{Complete: input.Lineage.Complete, Direction: direction, Depth: depth, Failure: artifactLineageFailure(input.Lineage.Failure), Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Lineage.Warnings...)},
	})
	if err != nil {
		return datasourceops.PullArtifactResult{}, err
	}
	canonicalPath, err := containDatasourceWorkspacePath(input.Workspace, result.CanonicalPath, "canonical path")
	if err != nil {
		return datasourceops.PullArtifactResult{}, err
	}
	lineagePath, err := containDatasourceWorkspacePath(input.Workspace, filepath.Join(input.Workspace, filepath.FromSlash(result.LineagePath)), "lineage path")
	if err != nil {
		return datasourceops.PullArtifactResult{}, err
	}
	// LineageStatus and CountsKnown are single-sourced from lineage completeness in the
	// pull action; leave LineageStatus unset here so the two fields cannot diverge.
	return datasourceops.PullArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: canonicalPath, BaselineFingerprint: result.BaselineFingerprint, LineagePath: lineagePath, CompositionStatus: result.CompositionStatus, ParentDataSourceURLs: append([]string(nil), result.ParentDataSourceURLs...), Warnings: append([]string(nil), result.Warnings...)}, nil
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

func (r datasourceArtifactReader) ReadDatasource(ctx context.Context, path string) (datasourceops.PublishArtifact, error) {
	item, err := r.manager.Read(ctx, path)
	return datasourceops.PublishArtifact{Path: r.displayPath, PayloadPath: item.PayloadPath, Filename: item.Filename, Name: item.Name, TableauID: item.TableauID, Fingerprint: item.Fingerprint, SourceEnvironment: item.SourceEnvironment, SourceSite: item.SourceSite, SourceProjectName: item.SourceProjectName, SourceProjectID: item.SourceProjectID, Size: item.Size, CompositionStatus: item.CompositionStatus, ParentDataSourceURLs: append([]string(nil), item.ParentDataSourceURLs...)}, err
}

type datasourcePublishAdapter struct {
	datasources             *resourcedatasource.Adapter
	projects                *resourceproject.Adapter
	changes                 *resourcedatasource.MutationAdapter
	runtime                 *runtimeDependencies
	environment, sourcePath string
	lifecycle               **publication
}

func (a datasourcePublishAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (datasourceops.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return datasourceops.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a datasourcePublishAdapter) FindDatasources(ctx context.Context, name, projectLUID string) ([]datasourceops.Record, error) {
	return a.datasources.FindDatasources(ctx, name, projectLUID)
}

func (a datasourcePublishAdapter) ResolvePublishedDatasource(ctx context.Context, name, projectLUID string) (datasourceops.Record, error) {
	if a.runtime != nil {
		fresh, err := newRemoteContentCommands(a.runtime).connect(ctx, a.environment, true)
		if err != nil {
			return datasourceops.Record{}, err
		}
		a.datasources = fresh.datasources
	}
	item, err := a.datasources.ResolvePublishedDatasource(ctx, name, projectLUID)
	if _, notVisible := errors.AsType[*resourcedatasource.PublishedDatasourceNotVisibleError](err); notVisible {
		return datasourceops.Record{}, datasourceops.PublishErrPublishedDatasourceNotVisible
	}
	return item, err
}

func (a datasourcePublishAdapter) Prepare(ctx context.Context, input datasourceops.PublishRequest) (datasourceops.PreparedPublish, error) {
	mode, err := datasourcePublishMode(input.Mode)
	if err != nil {
		return nil, err
	}
	request := tableaudatasource.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Mode: mode, ParentDataSourceURLs: append([]string(nil), input.ParentDataSourceURLs...), AsJob: input.AsJob}
	if a.runtime != nil {
		p, err := a.runtime.publication(ctx, a.environment, "datasource", a.sourcePath, input.ProjectLUID, input.Name)
		if err != nil {
			return nil, err
		}
		if a.lifecycle != nil {
			*a.lifecycle = p
		}
		request.AsJob, request.Accepted = p.asJob, p.acceptDatasource
	}
	prepared, err := a.changes.PrepareDatasource(ctx, request)
	if err != nil {
		return nil, err
	}
	return preparedDatasourcePublish{prepared: prepared}, nil
}

func datasourcePublishMode(mode datasourceops.Mode) (tableaudatasource.PublishMode, error) {
	switch mode {
	case datasourceops.ModeCreate:
		return tableaudatasource.PublishCreate, nil
	case datasourceops.ModeOverwrite:
		return tableaudatasource.PublishOverwrite, nil
	case datasourceops.ModeAppend:
		return tableaudatasource.PublishAppend, nil
	case datasourceops.ModeReplace:
		return tableaudatasource.PublishReplace, nil
	default:
		return "", errors.New("unsupported datasource publish mode")
	}
}

type preparedDatasourcePublish struct {
	prepared tableaudatasource.PreparedPublish
}

func (p preparedDatasourcePublish) Commit(ctx context.Context) (datasourceops.PublishResult, error) {
	progress.SetLabel(ctx, "Uploading and submitting datasource")
	result, err := p.prepared.Commit(ctx)
	return datasourceops.PublishResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, ProjectLUID: result.ProjectLUID, JobID: result.JobID, TableauRequestID: result.TableauRequestID, ReceiptPath: result.ReceiptPath}, err
}

func (a datasourceMutationAdapter) DeleteDatasource(ctx context.Context, luid string) (datasourceops.DeleteResult, error) {
	result, err := a.changes.DeleteDatasource(ctx, luid)
	return datasourceops.DeleteResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, TableauRequestID: result.TableauRequestID}, err
}
