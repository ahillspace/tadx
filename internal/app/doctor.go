package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	doctorrun "github.com/ahillspace/tadx/actions/doctor/run"
	"github.com/ahillspace/tadx/internal/artifact"
	corecatalog "github.com/ahillspace/tadx/internal/catalog"
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
		Catalog:       commands,
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
		return doctorrun.ConfigurationState{Present: true}, err
	}
	_, err = configuration.ResolveEnvironment(scope.Environment)
	return doctorrun.ConfigurationState{Present: true, Valid: true, EnvironmentResolved: err == nil}, nil
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
	return doctorrun.PATState{
		ReferencesConfigured:    environment.Auth.PATNameEnv != "" && environment.Auth.PATSecretEnv != "",
		NameVariablePresent:     namePresent && name != "",
		SecretVariablePresent:   secretPresent && secret != "",
		StoredCredentialPresent: environment.Auth.CredentialRef != "",
	}, nil
}

func (c *doctorCommands) CheckConnectivity(ctx context.Context, scope doctorrun.Scope) (doctorrun.ConnectivityState, error) {
	_, err := c.runtime.tableauConnection(ctx, scope.Environment, false)
	if err != nil {
		return doctorrun.ConnectivityState{}, err
	}
	return doctorrun.ConnectivityState{Reachable: true, Authenticated: true}, nil
}

func (c *doctorCommands) CheckCatalog(ctx context.Context, scope doctorrun.Scope) (doctorrun.CatalogState, error) {
	_, environment, err := c.runtime.environment(scope.Environment, false)
	if err != nil {
		return doctorrun.CatalogState{}, err
	}
	database := filepath.Join(filepath.Dir(c.runtime.configPath), "catalog", "catalog.sqlite")
	if _, err := os.Stat(database); errors.Is(err, os.ErrNotExist) {
		return doctorrun.CatalogState{}, nil
	} else if err != nil {
		return doctorrun.CatalogState{}, err
	}
	status, err := corecatalog.NewStore(filepath.Dir(c.runtime.configPath), c.runtime.now).Status(ctx, corecatalog.Selection{Environment: environment.Alias, Site: environment.SiteContentURL, SiteSelected: true})
	if err != nil {
		return doctorrun.CatalogState{Present: true}, err
	}
	return doctorrun.CatalogState{Present: true, Complete: status.Complete, Stale: status.Stale}, nil
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
