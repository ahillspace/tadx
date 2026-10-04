package app

import (
	"context"
	"os"

	doctor "github.com/ahillspace/tadx/actions/doctor"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/workspace"
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
		Cache:        doctor.NewCacheProbe(func() string { return runtime.configPath }, probes),
		Workspace:    doctor.NewWorkspaceProbe(probes),
	})
}

func (c *doctorRuntime) CheckConnectivity(ctx context.Context, scope doctor.Scope) error {
	_, err := c.runtime.tableauConnection(ctx, scope.Environment, false)
	return err
}

func (c *doctorRuntime) ResolveCacheTarget(scope doctor.Scope) (doctor.CacheTarget, error) {
	_, environment, err := c.runtime.environment(scope.Environment, false)
	if err != nil {
		return doctor.CacheTarget{}, err
	}
	return doctor.CacheTarget{
		Store:     c.runtime.cacheStore(environment),
		Selection: cache.Selection{Environment: environment.Alias, Site: environment.SiteContentURL, SiteSelected: true},
	}, nil
}

func (c *doctorRuntime) ResolveWorkspaceTarget(ctx context.Context, scope doctor.Scope) (workspace.Record, error) {
	return (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, scope.Workspace, scope.Environment)
}
