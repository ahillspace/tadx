package app

import (
	"context"
	"time"

	group "github.com/ahillspace/tadx/actions/admin/group"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
)

type adminGroupProvider struct{ commands *remoteAdminCommands }

func (p adminGroupProvider) CacheTarget(alias string) (group.Target, error) {
	environment, site, err := p.commands.resolveCacheTarget(alias)
	return group.Target{Environment: environment, Site: site}, err
}

func (p adminGroupProvider) CachedList(target group.Target) group.CachedListReader {
	return resourceadmin.NewCachedGroupListPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (p adminGroupProvider) CachedInspect(target group.Target) group.CachedInspectResolver {
	return resourceadmin.NewCachedGroupInspectPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (p adminGroupProvider) cacheSupport() resourceadmin.CacheSupport {
	return resourceadmin.CacheSupport{CheckCapability: p.commands.runtime.checkManagedCapability, ReadError: cacheReadError, UnsupportedFilter: unsupportedCacheFilters, ReadSource: cacheReadSource, RecordSource: cacheRecordSource}
}

func (adminGroupProvider) LegacyInventoryCursor(cursor string) bool {
	return legacyInventorySnapshot(cursor)
}

func (adminGroupProvider) ListFilter(input group.ListInput) (string, error) {
	return tableauadmin.GroupListFilter(tableauadmin.ListGroupsRequest{Name: input.Name, Domain: input.Domain})
}

func (p adminGroupProvider) Open(ctx context.Context, alias, site, operation string, explicit bool) (group.LiveSession, error) {
	connection, err := p.commands.connect(ctx, alias, explicit)
	if err != nil {
		return group.LiveSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	target := group.Target{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	return group.LiveSession{Target: target, Ports: resourceadmin.GroupPorts{Adapter: connection.adapter}, Inventory: resourceadmin.InventoryPorts{Executor: connection.inventory, Store: p.commands.cacheStore(target.Environment), Environment: target.Environment, Site: target.Site, MaxConcurrency: connection.environment.CacheMaxConcurrency, Now: p.commands.runtime.now}}, nil
}

func (adminGroupProvider) ValidateComplete(all bool, source *readsource.Metadata) error {
	return inventory.ValidateAll(all, source)
}

func (p adminGroupProvider) Now() time.Time { return p.commands.runtime.now() }
