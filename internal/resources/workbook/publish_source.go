package workbook

import (
	"context"
	"errors"
	"path/filepath"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
)

// PublishSource selects and validates an exact native or managed workbook artifact.
// Workspace resolves only a logical workspace; artifact selection remains here.
type PublishSource struct {
	Manager   *artifact.WorkbookManager
	Workspace func(context.Context, string, string) (name, root string, err error)
}

func (s PublishSource) Open(ctx context.Context, input workbook.PublishInput) (workbook.PublishInput, workbook.ArtifactReader, string, error) {
	if input.File != "" {
		input.ArtifactPath = input.File
		reader := NativePublishReader{}
		if _, err := reader.ReadWorkbook(ctx, input.File); err != nil {
			return input, nil, "", workbookPublishSetupError("file", input, "Native workbook file is invalid.", "Select an existing .twb or .twbx file with --file.", err)
		}
		return input, reader, input.ArtifactPath, nil
	}
	name, root, err := s.Workspace(ctx, input.Workspace, input.Environment)
	if err != nil {
		return input, nil, "", workbookPublishSetupError("workspace", input, "Workbook workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	managed, err := artifact.Resolve(ctx, root, artifact.Selector{Path: input.ArtifactPath, Kind: "workbook", LUID: input.ArtifactID, Name: input.ArtifactName})
	if err != nil {
		if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
			return input, nil, "", artifact.MapResolutionError("workbook.publish", name, input.ArtifactID, err)
		}
		return input, nil, "", workbookPublishSetupError("artifact", input, "Workbook artifact resolution failed.", "Select one exact workspace-relative managed workbook artifact, then retry.", err)
	}
	input.ArtifactPath = filepath.Join(root, filepath.FromSlash(managed.Path))
	input.WorkspaceName = name
	if _, err := s.Manager.Read(ctx, input.ArtifactPath); err != nil {
		return input, nil, "", workbookPublishSetupError("artifact", input, "Workbook artifact read failed.", "Repair or pull the exact workbook artifact, then retry.", err)
	}
	return input, ManagedPublishReader{Manager: s.Manager, DisplayPath: managed.Path}, managed.Path, nil
}

func workbookPublishSetupError(stage string, input workbook.PublishInput, summary, fallback string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, fallback)
	return &errs.Error{ID: "workbook.publish." + stage, Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}
