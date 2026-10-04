package app

import (
	"path/filepath"

	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/config"
)

func (r *runtimeDependencies) cacheStore(environment config.Environment) *cache.Store {
	return cache.NewTargetStore(filepath.Dir(r.configPath), environment.URL, environment.SiteContentURL, r.now)
}

func (c *remoteContentCommands) cacheStore(alias string) *cache.Store {
	_, environment, _ := c.runtime.environment(alias, false)
	return c.runtime.cacheStore(environment)
}

func (c *remoteContentCommands) resolveCacheTarget(alias string) (string, string, error) {
	_, environment, err := c.runtime.environment(alias, false)
	if err != nil {
		return alias, "", err
	}
	return environment.Alias, environment.SiteContentURL, nil
}
