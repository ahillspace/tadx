package cache

import (
	"context"
	"errors"
	"slices"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

// Provider binds local target selection, policy, and generation mechanisms.
// It opens no authenticated executor while local input is invalid or in preview.
type Provider interface {
	ResolveEnvironment(string) (config.Environment, error)
	CheckScopes([]string) error
	Hydrator(config.Environment, bool) Hydrator
	StatusSource(config.Environment) StatusSource
}

// Service owns named local cache management operations.
type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }

func (s *Service) RefreshCache(ctx context.Context, input RefreshInput) (RefreshOutput, error) {
	requested, implicit, err := normalizeScopes(input.Scopes)
	if err != nil {
		return RefreshOutput{}, failure("cache.refresh.usage", errs.KindUsage, input, err.Error(), err)
	}
	// Collector defaults exclude permissions during preflight. The action
	// includes permissions, so its prerequisite is checked after target selection.
	if !input.Preview {
		preflight := append(slices.Clone(requested), implicit...)
		if len(input.Scopes) == 0 {
			preflight = slices.DeleteFunc(preflight, func(scope string) bool { return scope == "permissions" })
		}
		if err := s.provider.CheckScopes(preflight); err != nil {
			return RefreshOutput{}, err
		}
	}
	environment, err := s.resolve(input.Environment, input.Site, "cache.refresh")
	if err != nil {
		return RefreshOutput{}, err
	}
	input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	if input.Preview {
		return refreshValidated(ctx, nil, input, requested, implicit)
	}
	return refreshValidated(ctx, s.provider.Hydrator(environment, len(input.Scopes) == 0), input, requested, implicit)
}

func (s *Service) ReadCacheStatus(ctx context.Context, input StatusInput) (StatusOutput, error) {
	environment, err := s.resolve(input.Environment, input.Site, "cache.status")
	if err != nil {
		return StatusOutput{}, err
	}
	input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	return readStatus(ctx, s.provider.StatusSource(environment), input)
}

func (s *Service) resolve(alias, site, operation string) (config.Environment, error) {
	environment, err := s.provider.ResolveEnvironment(alias)
	if err != nil {
		return environment, cacheSetupError(operation+".setup", operation, alias, site, "Cache operation setup failed.", "Review the selected environment and cache configuration.", err)
	}
	if site != "" && site != environment.SiteContentURL {
		return environment, cacheSetupError(operation+".setup", operation, environment.Alias, site, "Cache source site does not match the selected environment.", "Choose the configured exact site, then retry.", errors.New("cache source site mismatch"))
	}
	return environment, nil
}

func cacheSetupError(id, operation, environment, site, summary, fallback string, cause error) error {
	retryable, advice := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}
