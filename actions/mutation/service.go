// Package mutation owns local site-consent status, policy, and persistence.
package mutation

import (
	"context"
	"errors"
	"maps"
	"os"
	"slices"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// Restriction reports whether administrator policy blocks remote mutations.
type Restriction func() bool

// Service owns consent policy for one selected configuration file.
type Service struct {
	path        func() string
	restriction Restriction
}

func New(path func() string, restriction Restriction) *Service {
	return &Service{path: path, restriction: restriction}
}

// ReadMutationStatus lists saved consent without authorizing an operation.
func (s *Service) ReadMutationStatus(ctx context.Context, alias string) (value.MutationStatus, error) {
	out := value.MutationStatus{Sites: []value.MutationConsent{}}
	appendSetting := func(setting value.MutationSetting) {
		out.Sites = append(out.Sites, value.MutationConsent{Environment: setting.Environment, Enabled: setting.Enabled, ServerURL: setting.ServerURL, SiteContentURL: setting.SiteContentURL, Source: setting.Source})
	}
	if alias != "" {
		setting, err := s.ReadMutationSetting(ctx, alias)
		if err != nil {
			return out, err
		}
		appendSetting(setting)
	} else {
		cfg, err := config.Load(s.path())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return out, err
		}
		for _, name := range slices.Sorted(maps.Keys(cfg.Environments)) {
			environment := cfg.Environments[name]
			environment.Alias = name
			setting, err := SiteSetting(cfg, environment)
			if err != nil {
				return out, err
			}
			appendSetting(setting)
		}
	}
	if s.restriction != nil && s.restriction() {
		out.Restriction = "Administrator-managed policy blocks remote mutations; enabled reports site consent only."
	}
	return out, nil
}

// ReadMutationSetting reads one environment's canonical site consent.
func (s *Service) ReadMutationSetting(_ context.Context, alias string) (value.MutationSetting, error) {
	cfg, err := config.Load(s.path())
	if err != nil {
		return value.MutationSetting{}, &errs.Error{ID: "configuration.load", Kind: errs.KindOperation, Operation: "configuration", Summary: "CLI settings could not be loaded.", Cause: err, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false)}
	}
	environment, err := cfg.ResolveEnvironment(alias)
	if err != nil {
		return value.MutationSetting{}, &errs.Error{ID: "environment.resolve", Kind: errs.KindUsage, Operation: "environment.resolve", Environment: alias, Summary: "A configured environment alias is required, not a Tableau site name or URL.", Cause: err, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Prerequisite: &errs.Prerequisite{Kind: "environment", Resource: alias, Summary: "Select a configured environment alias."}}
	}
	return SiteSetting(cfg, environment)
}

// SiteSetting projects the canonical saved consent for a resolved environment.
func SiteSetting(cfg config.Config, environment config.Environment) (value.MutationSetting, error) {
	server, err := config.CanonicalMutationServer(environment.URL)
	if err != nil {
		return value.MutationSetting{}, err
	}
	saved, err := cfg.MutationSetting(environment)
	if err != nil {
		return value.MutationSetting{}, err
	}
	state := value.MutationSetting{Source: "default_disabled", Scope: "site", Environment: environment.Alias, ServerURL: server, SiteContentURL: environment.SiteContentURL, Saved: saved}
	if saved != nil {
		state.Enabled = *saved
		state.Source = "saved_site_setting"
		state.SourceSetting = "site_mutations"
	}
	return state, nil
}

// WriteMutationSetting persists consent for one resolved canonical site.
func (s *Service) WriteMutationSetting(_ context.Context, alias string, enabled bool) (value.MutationSetting, error) {
	var environment config.Environment
	cfg, err := config.Update(s.path(), false, func(cfg config.Config) (config.Config, error) {
		var resolveErr error
		environment, resolveErr = cfg.ResolveWriteEnvironment(alias)
		if resolveErr != nil {
			return cfg, &errs.Error{ID: "mutation.set.environment", Kind: errs.KindUsage, Operation: "mutation.set", Environment: alias, Summary: "Select one configured environment before changing site consent.", Cause: resolveErr, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false)}
		}
		if err := cfg.SetMutationSetting(environment, enabled); err != nil {
			return cfg, err
		}
		return cfg, nil
	})
	if err != nil {
		return value.MutationSetting{}, err
	}
	state, err := SiteSetting(cfg, environment)
	state.Persisted = new(true)
	if err != nil {
		state.Saved = new(enabled)
		state.Source = "unavailable"
		state.Scope = "site"
		return state, &errs.Error{ID: "mutation.set.policy_unavailable", Kind: errs.KindOperation, Operation: "mutation.set", Summary: "The site setting was saved, but mutation policy could not be resolved.", Cause: err, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed, Retryable: errs.Bool(false), CorrectiveAction: "Inspect mutation status for the selected environment; do not repeat the save."}
	}
	return state, nil
}

// Policy resolves local site consent for command execution checks.
func (s *Service) Policy(alias string) (bool, string, error) {
	if alias == "" {
		cfg, err := config.Load(s.path())
		if errors.Is(err, os.ErrNotExist) {
			return false, "site_selection_required", nil
		}
		if err != nil {
			return false, "unavailable", err
		}
		if _, err := cfg.ResolveEnvironment(""); err != nil {
			return false, "site_selection_required", nil
		}
	}
	state, err := s.ReadMutationSetting(context.Background(), alias)
	return state.Enabled, state.Source, err
}
