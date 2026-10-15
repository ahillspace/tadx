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
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
)

type doctorRuntime struct {
	runtime *runtimeDependencies
}

func newDoctorAction(runtime *runtimeDependencies) *doctorrun.Action {
	probes := &doctorRuntime{runtime: runtime}
	return doctorrun.New(doctorrun.Dependencies{
		Configuration: probes,
		PAT:           probes,
		Connectivity:  probes,
		Cache:         probes,
		Workspace:     probes,
		Logging:       probes,
	})
}

func (c *doctorRuntime) CheckConfiguration(_ context.Context, scope doctorrun.Scope) (doctorrun.ConfigurationState, error) {
	configuration, err := config.Load(c.runtime.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return doctorrun.ConfigurationState{}, nil
	}
	if err != nil {
		return doctorrun.ConfigurationState{Present: true, Cause: err.Error(), ConfigPath: c.runtime.configPath}, err
	}
	state := doctorrun.ConfigurationState{Present: true}
	for _, warning := range configuration.ConfigurationWarnings() {
		finding := doctorrun.Check{ID: "config." + warning.Kind + "." + warning.Name, Status: doctorrun.StatusWarn, Summary: warning.Summary}
		switch warning.Kind {
		case "environment":
			finding.Status = doctorrun.StatusFail
			finding.CorrectiveAction = commandhint.Recovery(warning.Commands, warning.Explanation, c.runtime.configPath)
		case "workspace":
			finding.Status = doctorrun.StatusFail
			finding.CorrectiveAction = commandhint.Recovery(warning.Commands, warning.Explanation, c.runtime.configPath)
		default:
			finding.CorrectiveAction = "Use a TADX build that supports this configuration field."
		}
		state.Findings = append(state.Findings, finding)
	}
	_, err = configuration.ResolveEnvironment(scope.Environment)
	if err != nil {
		state.SelectionInvalid = true
		if invalid, ok := errors.AsType[*config.InvalidEnvironmentError](err); ok {
			state.SelectionPrerequisite = "config.environment." + invalid.Alias
		} else {
			state.SelectionPrerequisite = "config.selection"
			fix := "Run tadx env list, then select a valid environment with --environment or tadx env set-default."
			if corrective, ok := err.(interface{ CorrectiveAction() string }); ok {
				fix = corrective.CorrectiveAction()
			}
			state.Findings = append(state.Findings, doctorrun.Check{ID: state.SelectionPrerequisite, Status: doctorrun.StatusFail, Summary: err.Error(), CorrectiveAction: fix})
		}
	}
	return state, nil
}

func (c *doctorRuntime) CheckPATReferences(_ context.Context, scope doctorrun.Scope) (doctorrun.PATState, error) {
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
		ReferencesConfigured:    true,
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

func (c *doctorRuntime) CheckConnectivity(ctx context.Context, scope doctorrun.Scope) error {
	_, err := c.runtime.tableauConnection(ctx, scope.Environment, false)
	return err
}

func (c *doctorRuntime) CheckCache(ctx context.Context, scope doctorrun.Scope) (doctorrun.CacheState, error) {
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

func (c *doctorRuntime) CheckWorkspace(ctx context.Context, scope doctorrun.Scope) (doctorrun.WorkspaceState, error) {
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, scope.Workspace, scope.Environment)
	if err != nil {
		return doctorrun.WorkspaceState{}, err
	}
	state := doctorrun.WorkspaceState{Available: workspace.Available, ManifestValid: workspace.ManifestValid}
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

func (*doctorRuntime) CheckLogging(_ context.Context, _ doctorrun.Scope) (doctorrun.LoggingState, error) {
	value, enabled := os.LookupEnv("TADX_LOG_LEVEL")
	return doctorrun.LoggingState{Enabled: enabled, Valid: !enabled || strings.TrimSpace(value) != ""}, nil
}
