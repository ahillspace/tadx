package definition

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
)

type PullWorkspace struct{ Root, Name string }

type PullSession struct {
	Environment, Site, SiteLUID, ServerOrigin string
	Reader                                    PullReader
	Writer                                    PullWriter
}

type PullProvider interface {
	ResolveDefinitionWorkspace(context.Context, string, string, string) (PullWorkspace, error)
	OpenDefinitionPull(context.Context, string, string) (PullSession, error)
}

// PullPulseDefinition acquires a complete native definition after local validation.
func (s *Service) PullPulseDefinition(ctx context.Context, input PullInput) (PullOutput, error) {
	if err := pullValidateInput(input); err != nil {
		return PullOutput{}, err
	}
	if s == nil || s.ports.Pull == nil {
		return PullOutput{}, &errs.Error{ID: "pulse.definition.pull.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.definition.pull", Summary: "Pulse definition pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure Pulse definition pulling before retrying."}
	}
	workspace, err := s.ports.Pull.ResolveDefinitionWorkspace(ctx, input.Workspace, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Workspace, input.WorkspaceName = workspace.Root, workspace.Name
	session, err := s.ports.Pull.OpenDefinitionPull(ctx, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	input.SiteLUID, input.ServerOrigin = session.SiteLUID, session.ServerOrigin
	return runPull(ctx, session.Reader, session.Writer, input)
}
