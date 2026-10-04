package app

import (
	"context"
	"time"

	user "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
)

type adminUserProvider struct{ commands *remoteAdminCommands }

func (p adminUserProvider) CacheTarget(alias string) (user.Target, error) {
	environment, site, err := p.commands.resolveCacheTarget(alias)
	return user.Target{Environment: environment, Site: site}, err
}

func (p adminUserProvider) CachedList(target user.Target) user.CachedListReader {
	return resourceadmin.NewCachedUserListPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (p adminUserProvider) CachedInspect(target user.Target) user.CachedInspectResolver {
	return resourceadmin.NewCachedUserInspectPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (p adminUserProvider) cacheSupport() resourceadmin.CacheSupport {
	return resourceadmin.CacheSupport{CheckCapability: p.commands.runtime.checkManagedCapability, ReadError: cacheReadError, UnsupportedFilter: unsupportedCacheFilters, ReadSource: cacheReadSource, RecordSource: cacheRecordSource}
}

func (adminUserProvider) LegacyInventoryCursor(cursor string) bool {
	return legacyInventorySnapshot(cursor)
}

func (adminUserProvider) ListFilter(input user.ListInput) (string, error) {
	return tableauadmin.UserListFilter(tableauadmin.ListUsersRequest{Name: input.Name, SiteRole: input.SiteRole})
}

func (p adminUserProvider) Open(ctx context.Context, alias, site, operation string, explicit bool) (user.LiveSession, error) {
	connection, err := p.commands.connect(ctx, alias, explicit)
	if err != nil {
		return user.LiveSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	target := user.Target{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	ports := resourceadmin.UserPorts{Adapter: connection.adapter, CallerLUID: connection.callerLUID, ServerURL: connection.environment.URL}
	return user.LiveSession{Target: target, Ports: ports, Inventory: resourceadmin.InventoryPorts{Executor: connection.inventory, Store: p.commands.cacheStore(target.Environment), Environment: target.Environment, Site: target.Site, MaxConcurrency: connection.environment.CacheMaxConcurrency, Now: p.commands.runtime.now}}, nil
}

func (adminUserProvider) ValidateComplete(all bool, source *readsource.Metadata) error {
	return inventory.ValidateAll(all, source)
}

func (p adminUserProvider) Now() time.Time { return p.commands.runtime.now() }
