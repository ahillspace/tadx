package flow

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type PullWorkspace struct{ Root, Name string }

type PullPreviewer interface {
	PreviewFlow(context.Context, PullInput, Record) (value.AcquisitionPlan, error)
}

type PullSession struct {
	Environment, Site, SiteLUID, ServerOrigin string
	Reader                                    PullReader
	Writer                                    PullWriter
	Previewer                                 PullPreviewer
}

type PullProvider interface {
	ResolveFlowWorkspace(context.Context, string, string, string) (PullWorkspace, error)
	OpenFlowPull(context.Context, string, string) (PullSession, error)
}

func (s *Service) PullFlow(ctx context.Context, input PullInput) (PullOutput, error) {
	if err := validatePullInput(input); err != nil {
		return PullOutput{}, err
	}
	if s == nil || s.ports.Pull == nil {
		return PullOutput{}, &errs.Error{ID: "flow.pull.unconfigured", Kind: errs.KindRuntime, Operation: "flow.pull", Summary: "Flow pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow pull before retrying."}
	}
	workspace, err := s.ports.Pull.ResolveFlowWorkspace(ctx, input.Workspace, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Workspace, input.WorkspaceName = workspace.Root, workspace.Name
	session, err := s.ports.Pull.OpenFlowPull(ctx, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Environment, input.Site, input.SiteLUID, input.ServerOrigin = session.Environment, session.Site, session.SiteLUID, session.ServerOrigin
	return pullValidated(ctx, session.Reader, session.Writer, session.Previewer, input)
}
