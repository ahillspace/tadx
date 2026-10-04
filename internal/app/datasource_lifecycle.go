package app

import (
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

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
				return datasourceops.PublishOutput{}, artifact.MapResolutionError("datasource.publish", workspace.Name, input.ArtifactID, err)
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
		out.Result.ReceiptPath, saveErr = lifecycle.Record(ctx, out.Result.JobID, out.Result.Status, out.Result.DatasourceLUID, out.Result.TableauRequestID, out.Result.Verification)
		err = errors.Join(err, saveErr)
	}
	if err == nil && out.Result != nil && out.Result.Status == "pending" && lifecycle != nil && !c.runtime.publicationNoWait() {
		contentbatch.DeferCompletion(ctx, func(ctx context.Context) (any, error) { return lifecycle.completeDatasource(ctx, action, out) })
	}
	return out, err
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
