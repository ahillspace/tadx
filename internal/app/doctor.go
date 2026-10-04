package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	doctor "github.com/ahillspace/tadx/actions/doctor"
	"github.com/ahillspace/tadx/internal/artifact"
	corecache "github.com/ahillspace/tadx/internal/cache"
)

type doctorRuntime struct {
	runtime *runtimeDependencies
}

func newDoctorAction(runtime *runtimeDependencies) *doctor.Service {
	probes := &doctorRuntime{runtime: runtime}
	return doctor.New(doctor.Dependencies{
		ConfigPath:   func() string { return runtime.configPath },
		LookupEnv:    os.LookupEnv,
		Connectivity: probes,
		Cache:        probes,
		Workspace:    probes,
	})
}

func (c *doctorRuntime) CheckConnectivity(ctx context.Context, scope doctor.Scope) error {
	_, err := c.runtime.tableauConnection(ctx, scope.Environment, false)
	return err
}

func (c *doctorRuntime) CheckCache(ctx context.Context, scope doctor.Scope) (doctor.CacheState, error) {
	_, environment, err := c.runtime.environment(scope.Environment, false)
	if err != nil {
		return doctor.CacheState{}, err
	}
	store := c.runtime.cacheStore(environment)
	database := filepath.Join(filepath.Dir(c.runtime.configPath), filepath.FromSlash(store.RelativePath()))
	if _, err := os.Stat(database); errors.Is(err, os.ErrNotExist) {
		return doctor.CacheState{}, nil
	} else if err != nil {
		return doctor.CacheState{}, err
	}
	status, err := store.Status(ctx, corecache.Selection{Environment: environment.Alias, Site: environment.SiteContentURL, SiteSelected: true})
	if err != nil {
		return doctor.CacheState{Present: true}, err
	}
	return doctor.CacheState{Present: true, Complete: status.Complete, Stale: status.Stale}, nil
}

func (c *doctorRuntime) CheckWorkspace(ctx context.Context, scope doctor.Scope) (doctor.WorkspaceState, error) {
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, scope.Workspace, scope.Environment)
	if err != nil {
		return doctor.WorkspaceState{}, err
	}
	state := doctor.WorkspaceState{Available: workspace.Available, ManifestValid: workspace.ManifestValid}
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
