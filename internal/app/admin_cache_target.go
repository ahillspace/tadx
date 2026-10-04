package app

import "github.com/ahillspace/tadx/internal/cache"

func (c *remoteAdminCommands) cacheStore(alias string) *cache.Store {
	_, environment, _ := c.runtime.environment(alias, false)
	return c.runtime.cacheStore(environment)
}

func (c *remoteAdminCommands) resolveCacheTarget(alias string) (string, string, error) {
	_, environment, err := c.runtime.environment(alias, false)
	if err != nil {
		return alias, "", err
	}
	return environment.Alias, environment.SiteContentURL, nil
}
