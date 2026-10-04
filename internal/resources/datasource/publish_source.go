package datasource

import (
	"context"
	"errors"
	"path/filepath"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
)

// PublishSource selects and validates a native or managed datasource artifact.
type PublishSource struct {
	Manager   *artifact.DatasourceManager
	Workspace func(context.Context, string, string) (name, root string, err error)
}

func (s PublishSource) Open(ctx context.Context, input datasource.PublishInput) (datasource.PublishInput, datasource.ArtifactReader, string, error) {
	if input.File != "" {
		if _, err := artifact.ReadNative(ctx, input.File, "datasource"); err != nil {
			return input, nil, "", datasourcePublishSetupError("file", input, "Native datasource validation failed.", "Select a valid native datasource file, then retry.", err)
		}
		input.ArtifactPath = input.File
		input.SourceDefaulted = false
		return input, NativePublishReader{}, input.ArtifactPath, nil
	}
	name, root, err := s.Workspace(ctx, input.Workspace, input.Environment)
	if err != nil {
		return input, nil, "", datasourcePublishSetupError("workspace", input, "Datasource workspace resolution failed.", "Select or configure the logical workspace containing the exact managed datasource artifact, then retry.", err)
	}
	managed, err := artifact.Resolve(ctx, root, artifact.Selector{Kind: "datasource", Path: input.ArtifactPath, LUID: input.ArtifactID, Name: input.ArtifactName})
	if err != nil {
		if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
			return input, nil, "", artifact.MapResolutionError("datasource.publish", name, input.ArtifactID, err)
		}
		return input, nil, "", datasourcePublishSetupError("artifact", input, "Datasource artifact resolution failed.", "Select one exact workspace-relative managed datasource artifact, then retry.", err)
	}
	absolutePath := filepath.Join(root, filepath.FromSlash(managed.Path))
	input.WorkspaceName = name
	if _, err := s.Manager.Read(ctx, absolutePath); err != nil {
		return input, nil, "", datasourcePublishSetupError("artifact", input, "Datasource artifact read failed.", "Repair or pull the exact datasource artifact, then retry.", err)
	}
	input.ArtifactPath = absolutePath
	input.SourceDefaulted = false
	return input, ManagedPublishReader{Manager: s.Manager, DisplayPath: managed.Path}, managed.Path, nil
}

func datasourcePublishSetupError(stage string, input datasource.PublishInput, summary, fallback string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, fallback)
	return &errs.Error{ID: "datasource.publish." + stage, Kind: errs.KindOperation, Operation: "datasource.publish", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}
