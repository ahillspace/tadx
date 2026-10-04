package app

import (
	"context"
	"time"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
)

type flowReadProvider struct {
	commands *remoteContentCommands
	resourceflow.ListFilterPort
}

func (p flowReadProvider) CacheTarget(alias string) (flow.ReadTarget, error) {
	environment, site, err := p.commands.resolveCacheTarget(alias)
	return flow.ReadTarget{Environment: environment, Site: site}, err
}
func (p flowReadProvider) CachedList(target flow.ReadTarget) flow.CachedListReader {
	return resourceflow.NewCachedListPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}
func (p flowReadProvider) CachedInspect(target flow.ReadTarget) flow.CachedInspectResolver {
	return resourceflow.NewCachedInspectPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}
func (p flowReadProvider) OpenFlowRead(ctx context.Context, alias, site, operation string) (flow.ReadSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return flow.ReadSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	target := flow.ReadTarget{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	ports := resourceflow.ReadPorts{Adapter: connection.flows}
	collection := resourceflow.InventoryPorts{Executor: connection.inventory, Store: func() *cache.Store { return p.commands.cacheStore(target.Environment) }, Environment: target.Environment, Site: target.Site, MaxConcurrency: connection.environment.CacheMaxConcurrency, Now: p.commands.runtime.now}
	return flow.ReadSession{ReadTarget: target, Reader: ports, Resolver: ports, Inventory: collection}, nil
}
func (p flowReadProvider) cacheSupport() resourceflow.CacheSupport {
	return resourceflow.CacheSupport{ReadError: inventory.CacheReadError, UnsupportedFilters: inventory.UnsupportedCacheFilters, ReadSource: inventory.CacheReadSource, RecordSource: inventory.CacheRecordSource}
}
func (p flowReadProvider) Now() time.Time { return p.commands.runtime.now() }
