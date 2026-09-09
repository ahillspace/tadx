package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/config"
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
	}
	if r.mutationOverride != nil {
		raw, present := r.mutationOverride()
		if present {
			if raw != "0" && raw != "1" {
				return value.MutationSetting{}, fmt.Errorf("TADX_ENABLE_MUTATIONS must be 0 or 1 when set")
			}
			state.Enabled = raw == "1"
			state.Source = "process_environment"
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
	return r.ReadMutationSetting(ctx)
}
func (r *runtimeDependencies) mutationPolicy() (bool, string, error) {
	state, err := r.ReadMutationSetting(context.Background())
	return state.Enabled, state.Source, err
}
