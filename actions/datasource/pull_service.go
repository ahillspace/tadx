package datasource

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type PullWorkspace struct{ Root, Name string }

type PullPreviewer interface {
	PreviewDatasource(context.Context, PullInput, Record) (value.AcquisitionPlan, error)
}

type PullSession struct {
	Environment, Site, SiteLUID, ServerOrigin string
	Reader                                    PullReader
	Writer                                    ArtifactWriter
	Previewer                                 PullPreviewer
}

type PullProvider interface {
	ResolveDatasourceWorkspace(context.Context, string, string, string) (PullWorkspace, error)
	OpenDatasourcePull(context.Context, string, string) (PullSession, error)
}

func (s *Service) PullDatasource(ctx context.Context, input PullInput) (PullOutput, error) {
	if err := validatePullInput(input); err != nil {
		return PullOutput{}, err
	}
	if s == nil || s.ports.Pull == nil {
		return PullOutput{}, &errs.Error{ID: "datasource.pull.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.pull", Summary: "Datasource pull is not configured.", Retryable: new(false), CorrectiveAction: "Configure datasource pull before retrying."}
	}
	workspace, err := s.ports.Pull.ResolveDatasourceWorkspace(ctx, input.Workspace, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Workspace, input.WorkspaceName = workspace.Root, workspace.Name
	session, err := s.ports.Pull.OpenDatasourcePull(ctx, input.Environment, input.Site)
	if err != nil {
		return PullOutput{}, err
	}
	input.Environment, input.Site, input.SiteLUID, input.ServerOrigin = session.Environment, session.Site, session.SiteLUID, session.ServerOrigin
	return pullValidated(ctx, session.Reader, session.Writer, session.Previewer, input)
}
