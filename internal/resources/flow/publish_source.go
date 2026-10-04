package flow

import (
	"context"
	"errors"
	"path/filepath"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
)

// PublishSource selects and validates a native or managed flow artifact.
type PublishSource struct {
	Manager   *artifact.FlowManager
	Workspace func(context.Context, string, string) (name, root string, err error)
}

func (s PublishSource) Open(ctx context.Context, input flow.PublishInput) (flow.PublishInput, flow.ArtifactReader, string, error) {
	if input.File != "" {
		if _, err := artifact.ReadNative(ctx, input.File, "flow"); err != nil {
			return input, nil, "", flowPublishSetupError("file", input, "Native flow validation failed.", "Select a valid native flow file, then retry.", err)
		}
		input.ArtifactPath = input.File
		return input, NativePublishReader{}, input.ArtifactPath, nil
	}
	name, root, err := s.Workspace(ctx, input.Workspace, input.Environment)
	if err != nil {
		return input, nil, "", flowPublishSetupError("workspace", input, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	managed, err := artifact.Resolve(ctx, root, artifact.Selector{Kind: "flow", Path: input.ArtifactPath, LUID: input.ArtifactID, Name: input.ArtifactName})
	if err != nil {
		if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
			return input, nil, "", artifact.MapResolutionError("flow.publish", name, input.ArtifactID, err)
		}
		return input, nil, "", flowPublishSetupError("artifact", input, "Flow artifact resolution failed.", "Select one exact workspace-relative managed flow artifact, then retry.", err)
	}
	absolutePath := filepath.Join(root, filepath.FromSlash(managed.Path))
	input.WorkspaceName = name
	if _, err := s.Manager.Read(ctx, absolutePath); err != nil {
		return input, nil, "", flowPublishSetupError("artifact", input, "Flow artifact read failed.", "Repair or pull the exact flow artifact, then retry.", err)
	}
	input.ArtifactPath = absolutePath
	return input, ManagedPublishReader{Manager: s.Manager, DisplayPath: managed.Path}, managed.Path, nil
}

func flowPublishSetupError(stage string, input flow.PublishInput, summary, fallback string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, fallback)
	return &errs.Error{ID: "flow.publish." + stage, Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}
