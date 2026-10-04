package app

import (
	"context"
	"errors"

	cacheops "github.com/ahillspace/tadx/actions/cache"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/inventory"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

// cacheProvider binds the action owner to invocation-scoped local and remote dependencies.
type cacheProvider struct{ runtime *runtimeDependencies }

func (p cacheProvider) ResolveEnvironment(alias string) (config.Environment, error) {
	_, environment, err := p.runtime.environment(alias, false)
	return environment, err
}

func (p cacheProvider) CheckScopes(scopes []string) error {
	return inventory.CheckScopeNames(p.runtime.checkManagedCapability, scopes)
}

func (p cacheProvider) Hydrator(environment config.Environment, checkDefault bool) cacheops.Hydrator {
	hydrator := inventory.GenerationHydrator{
		Store: p.runtime.cacheStore(environment), Now: p.runtime.now,
		ExecutorFor: func(ctx context.Context, alias, site string) (tableaucache.Executor, error) {
			connection, err := p.runtime.tableauConnection(ctx, alias, false)
			if err != nil {
				return nil, remoteSetupError("cache.refresh", alias, site, connection.environment, err)
			}
			if connection.environment.SiteContentURL != site {
				return nil, errors.New("cache refresh authenticated to a different site")
			}
			return tableaucache.AuthenticatedExecutor{
				Transport: connection.transport, Session: connection.session,
				ServerURL: connection.environment.URL, SiteLUID: connection.session.SiteLUID(),
			}, nil
		},
		NewRunner: func(executor tableaucache.Executor) (inventory.GenerationRunner, error) {
			return tableaucache.NewEngine(executor, tableaucache.Config{MaxConcurrency: environment.CacheMaxConcurrency})
		},
	}
	if checkDefault {
		hydrator.CheckScope = p.runtime.checkManagedCapability
	}
	return hydrator
}

func (p cacheProvider) StatusSource(environment config.Environment) cacheops.StatusSource {
	return inventory.StatusSource{Store: p.runtime.cacheStore(environment)}
}
