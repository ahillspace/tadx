package workbook

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type PullWorkspace struct{ Root, Name string }

type PullPreviewer interface {
	PreviewWorkbook(context.Context, PullInput, Record, []PublishedDatasource) (value.AcquisitionPlan, error)
}

type PullSession struct {
	Environment, Site, SiteLUID, ServerOrigin string
	Reader                                    PullReader
	Writer                                    ArtifactWriter
	Previewer                                 PullPreviewer
}

type PullProvider interface {
	ResolveWorkbookWorkspace(context.Context, string, string, string) (PullWorkspace, error)
	OpenWorkbookPull(context.Context, string, string) (PullSession, error)
}

func (s *Service) PullWorkbook(ctx context.Context, input PullInput) (PullOutput, error) {
	if err := validatePullInput(input); err != nil {
		return PullOutput{}, err
	}
	if s == nil || s.ports.Pull == nil {
		return PullOutput{}, &errs.Error{ID: "workbook.pull.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.pull", Summary: "Workbook pull is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook pulling before retrying."}
	}
	workspace, err := s.ports.Pull.ResolveWorkbookWorkspace(ctx, input.Workspace, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Workspace, input.WorkspaceName = workspace.Root, workspace.Name
	session, err := s.ports.Pull.OpenWorkbookPull(ctx, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Environment, input.Site, input.SiteLUID, input.ServerOrigin = session.Environment, session.Site, session.SiteLUID, session.ServerOrigin
	return pullValidated(ctx, session.Reader, session.Writer, session.Previewer, input)
}
