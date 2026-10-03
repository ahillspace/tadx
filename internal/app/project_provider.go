package app

import (
	"context"
	"time"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/readsource"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

// projectProvider constructs project ports only after the action validates input.
type projectProvider struct{ commands *remoteContentCommands }

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
	return resourceproject.CacheSupport{ReadError: cacheReadError, UnsupportedFilters: unsupportedCacheFilters, ReadSource: cacheReadSource, RecordSource: cacheRecordSource}
}

func (p projectProvider) LegacyInventoryCursor(value string) bool {
	return legacyInventorySnapshot(value)
}

func (p projectProvider) ListFilter(input projectops.ListInput) (string, error) {
	return resourceproject.ListFilter(input)
}

func (p projectProvider) ValidateComplete(all bool, source *readsource.Metadata) error {
	return validateInventoryAll(all, source)
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
		Inventory: projectInventoryPort{commands: p.commands, connection: connection},
	}, nil
}

type projectInventoryPort struct {
	commands   *remoteContentCommands
	connection remoteConnection
}

func (p projectInventoryPort) CollectProjects(ctx context.Context, filter string, observedAt time.Time) (projectops.CollectedList, error) {
	environment, site := p.connection.environment.Alias, p.connection.environment.SiteContentURL
	inventory, err := collectResourceInventory(ctx, p.connection.inventory, p.commands.cacheStore(environment), tableaucache.ScopeProjects, environment, site, observedAt, inventoryCollectionOptions{MaxConcurrency: p.connection.environment.CacheMaxConcurrency, Filter: filter})
	if err != nil {
		return projectops.CollectedList{}, inventoryRefreshError("project.list", environment, site, err)
	}
	reader := resourceproject.InventoryListPort{Entries: inventory.entries, RequestID: finalRequestID(inventory.requestIDs), AllowContinuation: true}
	source, help := inventory.sourceAndHelp(observedAt, p.commands.runtime.now)
	return projectops.CollectedList{Reader: reader, RequestID: reader.RequestID, Source: source, Help: help}, nil
}

func (p projectInventoryPort) PublishProjectInspect(_ context.Context, output projectops.InspectOutput, observedAt time.Time) {
	entry, err := resourceproject.InspectEntry(output, observedAt)
	if err == nil {
		writeThrough(p.commands.cacheStore(output.Environment), []cache.ResourceEntry{entry})
	}
}
