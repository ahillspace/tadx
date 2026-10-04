package lineage

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
)

// Workspace identifies the local destination selected for one pull.
type Workspace struct {
	Root string
	Name string
}

// WorkspaceResolver selects a configured workspace for the requested environment.
type WorkspaceResolver interface {
	ResolveLineageWorkspace(context.Context, string, string) (Workspace, error)
}

// ReadSession contains the canonical source and narrow ports for one pull.
type ReadSession struct {
	Environment  string
	Site         string
	ServerOrigin string
	SiteLUID     string
	Resolver     Resolver
	Reader       Reader
	Writer       Writer
	Previewer    Previewer
}

// Provider opens the selected source only after local input and workspace validation.
// It returns the resolved environment and site even when setup fails.
type Provider interface {
	OpenLineage(context.Context, string) (ReadSession, error)
}

// Service owns lineage pull sequencing and its observable setup failures.
type Service struct {
	workspaces WorkspaceResolver
	provider   Provider
}

// New constructs the lineage owner from its workspace and remote dependencies.
func New(workspaces WorkspaceResolver, provider Provider) *Service {
	return &Service{workspaces: workspaces, provider: provider}
}

// PullLineage validates, resolves, and captures one bounded graph.
func (s *Service) PullLineage(ctx context.Context, input Input) (Output, error) {
	validated, err := NormalizeInput(input)
	if err != nil {
		return Output{}, err
	}
	if s == nil || s.workspaces == nil || s.provider == nil {
		return Output{}, unconfigured()
	}
	workspace, err := s.workspaces.ResolveLineageWorkspace(ctx, validated.Workspace, validated.Environment)
	if err != nil {
		return Output{}, setupError("lineage.pull.workspace", validated.Environment, validated.Site, "Lineage workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	validated.Workspace = workspace.Root
	validated.WorkspaceName = workspace.Name
	session, err := s.provider.OpenLineage(ctx, validated.Environment)
	if err != nil {
		environment, site := validated.Environment, validated.Site
		if session.Environment != "" {
			environment = session.Environment
			// A resolved environment's empty site is authoritative.
			site = session.Site
		} else if session.Site != "" {
			site = session.Site
		}
		return Output{}, setupError("lineage.pull.setup", environment, site, "Tableau operation setup failed.", "Review the selected environment, site, and PAT configuration.", err)
	}
	validated.Environment, validated.Site = session.Environment, session.Site
	validated.ServerOrigin, validated.SiteLUID = session.ServerOrigin, session.SiteLUID
	return executeValidated(ctx, session.Resolver, session.Reader, session.Writer, session.Previewer, validated)
}

func setupError(id, environment, site, summary, fallback string, cause error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "lineage.pull", Environment: environment, Site: site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}

func unconfigured() error {
	return &errs.Error{ID: "lineage.pull.unconfigured", Kind: errs.KindRuntime, Operation: "lineage.pull", Summary: "Lineage pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure lineage pull before retrying."}
}
