package app

import (
	"context"
	"time"

	definition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/resources/pulse"
)

type pulseDefinitionReadProvider struct{ commands *pulseCommands }

var _ definition.ReadProvider = pulseDefinitionReadProvider{}

func (p pulseDefinitionReadProvider) CacheTarget(alias string) (definition.ReadTarget, error) {
	_, environment, err := p.commands.runtime.environment(alias, false)
	return definition.ReadTarget{Environment: environment.Alias, Site: environment.SiteContentURL}, err
}

func (p pulseDefinitionReadProvider) CachedList(target definition.ReadTarget) definition.CachedListPort {
	return &pulse.CachedDefinitionListPort{Store: p.commands.cacheStore(target.Environment), Environment: target.Environment, Site: target.Site, Support: pulseCacheSupport()}
}

func (p pulseDefinitionReadProvider) CachedInspect(target definition.ReadTarget) definition.CachedInspectPort {
	return &pulse.CachedDefinitionInspectPort{Store: p.commands.cacheStore(target.Environment), Environment: target.Environment, Site: target.Site, Support: pulseCacheSupport()}
}

func (c *pulseCommands) cacheStore(alias string) *cache.Store {
	_, environment, _ := c.runtime.environment(alias, false)
	return c.runtime.cacheStore(environment)
}

func pulseCacheSupport() pulse.CacheSupport {
	return pulse.CacheSupport{ReadError: inventory.CacheReadError, ReadSource: inventory.CacheReadSource, RecordSource: inventory.CacheRecordSource}
}

func (p pulseDefinitionReadProvider) Open(ctx context.Context, alias, site, operation string) (definition.ReadSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return definition.ReadSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	target := definition.ReadTarget{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	store := p.commands.runtime.cacheStore(connection.environment)
	return definition.ReadSession{
		ReadTarget: target,
		List:       &pulse.DefinitionListPort{Client: connection.client, Store: store, Environment: target.Environment, Site: target.Site, Now: p.commands.runtime.now},
		Inspect:    &pulse.DefinitionInspectPort{Client: connection.client, Store: store, Environment: target.Environment, Site: target.Site, Now: p.commands.runtime.now},
	}, nil
}

func (p pulseDefinitionReadProvider) CacheSetupError(operation, alias string, err error) error {
	return capabilitySetupError(operation+".cache.setup", operation, alias, "", "Cache Pulse definition setup failed.", "Verify the selected environment and cache configuration.", err)
}

func (p pulseDefinitionReadProvider) Now() time.Time { return p.commands.runtime.now() }
