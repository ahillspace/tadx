package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"os"
)

func (r *runtimeDependencies) ReadMutationSetting(_ context.Context) (value.MutationSetting, error) {
	cfg, err := config.Load(r.configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return value.MutationSetting{}, err
	}
	state := value.MutationSetting{Source: "default_disabled", Scope: "user", Saved: cfg.MutationsEnabled}
	if cfg.MutationsEnabled != nil {
		state.Enabled = *cfg.MutationsEnabled
		state.Source = "saved_user_setting"
		state.SourceSetting = "mutations_enabled"
	}
	if r.mutationOverride != nil {
		raw, present := r.mutationOverride()
		if present {
			if raw != "0" && raw != "1" {
				return value.MutationSetting{}, fmt.Errorf("TADX_ENABLE_MUTATIONS must be 0 or 1 when set")
			}
			state.Enabled = raw == "1"
			state.Source = "process_environment"
			state.SourceSetting = "TADX_ENABLE_MUTATIONS"
			state.Scope = "process"
		}
	}
	return state, nil
}
func (r *runtimeDependencies) WriteMutationSetting(ctx context.Context, enabled bool) (value.MutationSetting, error) {
	_, err := config.Update(r.configPath, true, func(cfg config.Config) (config.Config, error) { cfg.MutationsEnabled = &enabled; return cfg, nil })
	if err != nil {
		return value.MutationSetting{}, err
	}
	state, err := r.ReadMutationSetting(ctx)
	state.Persisted = new(true)
	if err != nil {
		state.Saved = new(enabled)
		state.Source = "unavailable"
		state.Scope = "user"
		return state, &errs.Error{ID: "mutation.set.policy_unavailable", Kind: errs.KindOperation, Operation: "mutation.set", Summary: "The user setting was saved, but effective mutation policy could not be resolved.", Cause: err, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed, Retryable: errs.Bool(false), CorrectiveAction: "Retain the saved setting. Correct the effective override and inspect mutation status; do not repeat the save."}
	}
	return state, nil
}
func (r *runtimeDependencies) mutationPolicy() (bool, string, error) {
	state, err := r.ReadMutationSetting(context.Background())
	return state.Enabled, state.Source, err
}
