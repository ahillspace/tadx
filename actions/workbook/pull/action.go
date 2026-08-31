package pull

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Reader owns remote workbook resolution and download.
type Reader interface {
	ResolveWorkbook(context.Context, identity.Selector) (Workbook, error)
	DownloadWorkbook(context.Context, string, *bool) (Download, error)
}

// ArtifactWriter owns canonical local persistence and dirty-state protection.
type ArtifactWriter interface {
	WriteWorkbook(context.Context, Artifact) (ArtifactResult, error)
}

// Action orchestrates workbook.pull.
type Action struct {
	reader Reader
	writer ArtifactWriter
}

// New creates workbook.pull.
func New(reader Reader, writer ArtifactWriter) *Action {
	return &Action{reader: reader, writer: writer}
}

// Execute resolves, downloads, and materializes one workbook artifact.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, &errs.Error{ID: "workbook.pull.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.pull", Summary: "Workbook pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook pulling before retrying."}
	}
	selector := input.Selector
	if selector.LUID == "" && selector.Name == "" && selector.ProjectPath == "" {
		selector = identity.Selector{LUID: identity.LUID(input.LUID), Name: input.Name, ProjectPath: input.ProjectPath}
	}
	workbook, err := a.reader.ResolveWorkbook(ctx, selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workbook selector and target, then retry.")
		return Output{}, &errs.Error{ID: "workbook.resolve.failed", Kind: errs.KindOperation, Operation: "workbook.pull", Environment: input.Environment, Site: input.Site, Summary: "Workbook resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	download, err := a.reader.DownloadWorkbook(ctx, workbook.LUID, input.IncludeExtract)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review workbook access and the exact target, then retry.")
		requestID := download.TableauRequestID
		if requestID == "" {
			requestID = errs.TableauRequestID(err)
		}
		return Output{}, &errs.Error{ID: "workbook.download.failed", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook download failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: requestID}
	}
	artifact, err := a.writer.WriteWorkbook(ctx, Artifact{
		Workspace: input.Workspace, Filename: download.Filename, Content: download.Content,
		Name: workbook.Name, TableauID: workbook.LUID, Environment: input.Environment, Site: input.Site,
		ProjectName: workbook.ProjectPath, ProjectID: workbook.ProjectLUID, Overwrite: input.Overwrite, TableauRequestID: download.TableauRequestID,
	})
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve the local artifact conflict or use --overwrite after reviewing the current files.")
		return Output{}, &errs.Error{ID: "workbook.artifact.write", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook artifact write failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: download.TableauRequestID}
	}
	return Output{Status: "pulled", Workbook: workbook, Artifact: artifact, Warnings: artifact.Warnings, RequestID: download.TableauRequestID}, nil
}
