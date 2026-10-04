package app

import (
	"context"
	"os"

	authstatus "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/actions/env"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func newEnvironmentDependencies(runtime *runtimeDependencies) *envcli.Dependencies {
	service := env.New(env.NewConfigStore(func() string { return runtime.configPath }, tableaucache.DefaultMaxConcurrency))
	return &envcli.Dependencies{
		Lister: service.List, Getter: service.Get, Adder: service.Add,
		Updater: service.Update, Remover: service.Remove, DefaultSetter: service.SetDefault,
		Uses:   registryUses("env.profile.list", "env.profile.get", "env.profile.add", "env.profile.update", "env.profile.remove", "env.profile.set-default"),
		Shorts: registryShorts("env.profile.list", "env.profile.get", "env.profile.add", "env.profile.update", "env.profile.remove", "env.profile.set-default"),
	}
}

type authStatusResolver struct{ runtime *runtimeDependencies }

func (r authStatusResolver) Resolve(_ context.Context, alias string) (authstatus.StatusTarget, error) {
	configuration, environment, err := r.runtime.environment(alias, false)
	if err != nil {
		return authstatus.StatusTarget{}, err
	}
	return authstatus.StatusTargetFromConfig(configuration, environment), nil
}

type processEnvironment struct{}

func (processEnvironment) LookupEnv(name string) (string, bool) { return os.LookupEnv(name) }

func registryUses(ids ...string) map[string]string {
	result := make(map[string]string, len(ids))
	for _, id := range ids {
		result[id] = registryLeafUse(id)
	}
	return result
}
func registryShorts(ids ...string) map[string]string {
	result := make(map[string]string, len(ids))
	for _, id := range ids {
		result[id] = registryShort(id)
	}
	return result
}
