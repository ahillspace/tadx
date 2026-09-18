package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	doctorrun "github.com/ahillspace/tadx/actions/doctor/run"
	"github.com/ahillspace/tadx/internal/artifact"
	corecache "github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/config"
)

type doctorCommands struct {
	runtime *runtimeDependencies
	action  *doctorrun.Action
}

func newDoctorCommands(runtime *runtimeDependencies) *doctorCommands {
	commands := &doctorCommands{runtime: runtime}
	commands.action = doctorrun.New(doctorrun.Dependencies{
		Configuration: commands,
		PAT:           commands,
		Connectivity:  commands,
		Cache:         commands,
		Workspace:     commands,
		Logging:       commands,
	})
	return commands
}

func (c *doctorCommands) Execute(ctx context.Context, input doctorrun.Input) (doctorrun.Output, error) {
	return c.action.Execute(ctx, input)
}

func (c *doctorCommands) CheckConfiguration(_ context.Context, scope doctorrun.Scope) (doctorrun.ConfigurationState, error) {
	configuration, err := config.Load(c.runtime.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return doctorrun.ConfigurationState{}, nil
	}
	if err != nil {
		return doctorrun.ConfigurationState{Present: true, Cause: err.Error(), ConfigPath: c.runtime.configPath}, err
	}
	_, err = configuration.ResolveEnvironment(scope.Environment)
	state := doctorrun.ConfigurationState{Present: true, Valid: true, EnvironmentResolved: err == nil}
	if err != nil {
		state.Cause, state.ConfigPath = err.Error(), c.runtime.configPath
	}
	return state, err
}

func (c *doctorCommands) CheckPATReferences(_ context.Context, scope doctorrun.Scope) (doctorrun.PATState, error) {
	configuration, err := config.Load(c.runtime.configPath)
	if err != nil {
		return doctorrun.PATState{}, err
	}
	environment, err := configuration.ResolveEnvironment(scope.Environment)
	if err != nil {
		return doctorrun.PATState{}, err
	}
	name, namePresent := os.LookupEnv(environment.Auth.PATNameEnv)
	secret, secretPresent := os.LookupEnv(environment.Auth.PATSecretEnv)
	state := doctorrun.PATState{
		ReferencesConfigured:    environment.Auth.PATNameEnv != "" && environment.Auth.PATSecretEnv != "",
		NameVariablePresent:     namePresent && name != "",
		SecretVariablePresent:   secretPresent && secret != "",
		StoredCredentialPresent: environment.Auth.CredentialRef != "",
		NameVariable:            environment.Auth.PATNameEnv,
		SecretVariable:          environment.Auth.PATSecretEnv,
	}
	switch {
	case state.NameVariablePresent && state.SecretVariablePresent:
		state.Source = "environment"
	case state.NameVariablePresent != state.SecretVariablePresent:
		state.Source = "incomplete_environment"
	case state.StoredCredentialPresent:
		state.Source = "credential_store_reference"
	default:
		state.Source = "unavailable"
	}
	return state, nil
}

func (c *doctorCommands) CheckConnectivity(ctx context.Context, scope doctorrun.Scope) (doctorrun.ConnectivityState, error) {
	_, err := c.runtime.tableauConnection(ctx, scope.Environment, false)
	if err != nil {
		return doctorrun.ConnectivityState{}, err
	}
	return doctorrun.ConnectivityState{Reachable: true, Authenticated: true}, nil
}

func (c *doctorCommands) CheckCache(ctx context.Context, scope doctorrun.Scope) (doctorrun.CacheState, error) {
	_, environment, err := c.runtime.environment(scope.Environment, false)
	if err != nil {
		return doctorrun.CacheState{}, err
	}
	store := c.runtime.cacheStore(environment)
	database := filepath.Join(filepath.Dir(c.runtime.configPath), filepath.FromSlash(store.RelativePath()))
	if _, err := os.Stat(database); errors.Is(err, os.ErrNotExist) {
		return doctorrun.CacheState{}, nil
	} else if err != nil {
		return doctorrun.CacheState{}, err
	}
	status, err := store.Status(ctx, corecache.Selection{Environment: environment.Alias, Site: environment.SiteContentURL, SiteSelected: true})
	if err != nil {
		return doctorrun.CacheState{Present: true}, err
	}
	return doctorrun.CacheState{Present: true, Complete: status.Complete, Stale: status.Stale}, nil
}

func (c *doctorCommands) CheckWorkspace(ctx context.Context, scope doctorrun.Scope) (doctorrun.WorkspaceState, error) {
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, scope.Workspace, scope.Environment)
	if err != nil {
		return doctorrun.WorkspaceState{}, err
	}
	state := doctorrun.WorkspaceState{Selected: true, Available: workspace.Available, ManifestValid: workspace.ManifestValid}
	if !workspace.Available || !workspace.ManifestValid {
		return state, nil
	}
	inventory, err := artifact.Inventory(ctx, workspace.Root, artifact.InventoryOptions{Limit: 1000})
	if err != nil {
		return state, err
	}
	state.DirtyArtifacts = inventory.Dirty + inventory.Missing + inventory.Invalid
	return state, nil
}

func (*doctorCommands) CheckLogging(_ context.Context, _ doctorrun.Scope) (doctorrun.LoggingState, error) {
	value, enabled := os.LookupEnv("TADX_LOG_LEVEL")
	return doctorrun.LoggingState{Enabled: enabled, Valid: !enabled || strings.TrimSpace(value) != ""}, nil
}
