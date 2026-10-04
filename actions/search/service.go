package search

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Target struct{ Environment, Site string }

type Session struct {
	Target
	Source Source
}

// Provider constructs sources without deciding which workflow the input selects.
type Provider interface {
	CheckCapability(string) error
	Target(string) (Target, error)
	Cached(string) (Session, error)
	Complete(string) (Session, error)
	Live(context.Context, string) (Session, error)
}

type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }

func (s *Service) Execute(ctx context.Context, input Input) (Output, error) {
	types, err := ValidateInput(input)
	if err != nil {
		return Output{}, err
	}
	for _, kind := range types {
		if kind == "user" || kind == "group" {
			if err := s.provider.CheckCapability("admin." + kind + ".list"); err != nil {
				return Output{}, err
			}
		}
	}
	if input.Cursor != "" {
		target, err := s.provider.Target(input.Environment)
		if err != nil {
			return Output{}, err
		}
		input.Environment, input.Site, input.SiteResolved = target.Environment, target.Site, true
		if err := ValidateContinuation(input); err != nil {
			return Output{}, err
		}
	}
	if input.Cache {
		session, err := s.provider.Cached(input.Environment)
		if err != nil {
			return Output{}, setupError("search.cache.setup", input.Environment, input.Site, "Cache search setup failed.", "Review the selected environment and cache configuration.", err)
		}
		input.Environment = session.Environment
		if input.Site == "" {
			input.Site = session.Site
		}
		input.SiteResolved = true
		return Execute(ctx, session.Source, input, types)
	}
	if strings.TrimSpace(input.Terms) == "" && CompleteListSelector(input.Type) {
		session, err := s.provider.Complete(input.Environment)
		if err != nil {
			return Output{}, remoteSetupError(input, session.Target, err)
		}
		input.Environment, input.Site, input.SiteResolved = session.Environment, session.Site, true
		return Execute(ctx, session.Source, input, types)
	}
	session, err := s.provider.Live(ctx, input.Environment)
	if err != nil {
		return Output{}, remoteSetupError(input, session.Target, err)
	}
	input.Environment, input.Site, input.SiteResolved = session.Environment, session.Site, true
	return Execute(ctx, session.Source, input, types)
}

// CompleteListSelector identifies blank searches that retain public-list coverage.
func CompleteListSelector(selector string) bool {
	switch selector {
	case "content", "admin", "workbook", "datasource", "flow", "project", "user", "group":
		return true
	default:
		return false
	}
}

func remoteSetupError(input Input, target Target, cause error) error {
	environment, site := input.Environment, input.Site
	if target.Environment != "" {
		environment = target.Environment
	}
	if target.Site != "" || target.Environment != "" {
		site = target.Site
	}
	return setupError("search.setup", environment, site, "Tableau operation setup failed.", "Review the selected environment, site, and PAT configuration.", cause)
}

func setupError(id, environment, site, summary, fallback string, cause error) error {
	retryable, advice := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "search", Environment: environment, Site: site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}
