package app

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"os"
)

func (r *runtimeDependencies) ReadMutationSetting(_ context.Context, alias string) (value.MutationSetting, error) {
	cfg, environment, err := r.environment(alias, false)
	if err != nil {
		return value.MutationSetting{}, err
	}
	return siteMutationSetting(cfg, environment)
}
func siteMutationSetting(cfg config.Config, environment config.Environment) (value.MutationSetting, error) {
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
func (r *runtimeDependencies) WriteMutationSetting(_ context.Context, alias string, enabled bool) (value.MutationSetting, error) {
	var environment config.Environment
	cfg, err := config.Update(r.configPath, false, func(cfg config.Config) (config.Config, error) {
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
	state, err := siteMutationSetting(cfg, environment)
	state.Persisted = new(true)
	if err != nil {
		state.Saved = new(enabled)
		state.Source = "unavailable"
		state.Scope = "site"
		return state, &errs.Error{ID: "mutation.set.policy_unavailable", Kind: errs.KindOperation, Operation: "mutation.set", Summary: "The site setting was saved, but mutation policy could not be resolved.", Cause: err, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed, Retryable: errs.Bool(false), CorrectiveAction: "Inspect mutation status for the selected environment; do not repeat the save."}
	}
	return state, nil
}
func (r *runtimeDependencies) mutationPolicy(alias string) (bool, string, error) {
	if alias == "" {
		cfg, err := config.Load(r.configPath)
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
	state, err := r.ReadMutationSetting(context.Background(), alias)
	return state.Enabled, state.Source, err
}
