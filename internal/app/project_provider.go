package app

import (
	"context"
	"time"

	projectops "github.com/ahillspace/tadx/actions/project"
	inventorycore "github.com/ahillspace/tadx/internal/inventory"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
)

// projectProvider constructs project ports only after the action validates input.
type projectProvider struct {
	commands *remoteContentCommands
	resourceproject.ListFilterPort
}

var _ projectops.Provider = projectProvider{}

func (p projectProvider) CacheTarget(alias string) (projectops.Target, error) {
	environment, site, err := p.commands.resolveCacheTarget(alias)
	return projectops.Target{Environment: environment, Site: site}, err
}

func (p projectProvider) CachedList(target projectops.Target) projectops.CachedListReader {
	return resourceproject.NewCachedListPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, projectCacheSupport())
}

func (p projectProvider) CachedInspect(target projectops.Target) projectops.CachedInspectResolver {
	return resourceproject.NewCachedInspectPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, projectCacheSupport())
}

func projectCacheSupport() resourceproject.CacheSupport {
	return resourceproject.CacheSupport{ReadError: inventorycore.CacheReadError, UnsupportedFilters: inventorycore.UnsupportedCacheFilters, ReadSource: inventorycore.CacheReadSource, RecordSource: inventorycore.CacheRecordSource}
}

func (p projectProvider) Now() time.Time { return p.commands.runtime.now() }

func (p projectProvider) Open(ctx context.Context, alias, site, operation string, mutating bool) (projectops.LiveSession, error) {
	connection, err := p.commands.connect(ctx, alias, mutating)
	if err != nil {
		return projectops.LiveSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	create := resourceproject.NewCreatePort(connection.projects, connection.projectChanges)
	update := resourceproject.NewUpdatePort(connection.projects, connection.projectChanges)
	remove := resourceproject.NewDeletePort(connection.projects, connection.projectChanges)
	move := resourceproject.NewMovePort(connection.projects, connection.projectChanges)
	return projectops.LiveSession{
		Target: projectops.Target{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL},
		Ports: projectops.Ports{
			ListReader: resourceproject.ListPort{Adapter: connection.projects}, InspectResolver: resourceproject.InspectPort{Adapter: connection.projects},
			CreateResolver: create, Creator: create, UpdateResolver: update, Updater: update,
			DeleteResolver: remove, Deleter: remove, MoveResolver: move, Mover: move,
		},
		Inventory: resourceproject.InventoryPorts{Executor: connection.inventory, Store: p.commands.cacheStore, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, MaxConcurrency: connection.environment.CacheMaxConcurrency, Now: p.commands.runtime.now},
	}, nil
}
