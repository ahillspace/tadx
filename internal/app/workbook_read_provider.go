package app

import (
	"context"
	"time"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type workbookReadProvider struct{ commands *remoteContentCommands }

func (p workbookReadProvider) CacheTarget(alias string) (workbook.ReadTarget, error) {
	environment, site, err := p.commands.resolveCacheTarget(alias)
	return workbook.ReadTarget{Environment: environment, Site: site}, err
}

func (p workbookReadProvider) CachedList(target workbook.ReadTarget) workbook.CachedListReader {
	return resourceworkbook.NewCachedListPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (p workbookReadProvider) CachedInspect(target workbook.ReadTarget) workbook.CachedInspectResolver {
	return resourceworkbook.NewCachedInspectPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (workbookReadProvider) LegacyInventoryCursor(cursor string) bool {
	return legacyInventorySnapshot(cursor)
}

func (workbookReadProvider) ListFilter(input workbook.ListInput) (string, error) {
	return tableauworkbook.ListFilter(tableauworkbook.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
}

func (p workbookReadProvider) OpenWorkbookRead(ctx context.Context, alias, site, operation string) (workbook.ReadSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return workbook.ReadSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	target := workbook.ReadTarget{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	ports := resourceworkbook.ReadPorts{Adapter: connection.workbooks}
	collection := resourceworkbook.InventoryPorts{Adapter: connection.workbooks, Executor: connection.inventory, Store: func() *cache.Store { return p.commands.cacheStore(target.Environment) }, Environment: target.Environment, Site: target.Site, MaxConcurrency: connection.environment.CacheMaxConcurrency, Now: p.commands.runtime.now}
	return workbook.ReadSession{ReadTarget: target, Reader: ports, Resolver: ports, Inventory: collection}, nil
}

func (p workbookReadProvider) cacheSupport() resourceworkbook.CacheSupport {
	return resourceworkbook.CacheSupport{ReadError: cacheReadError, UnsupportedFilters: unsupportedCacheFilters, ReadSource: cacheReadSource, RecordSource: cacheRecordSource}
}

func (workbookReadProvider) ValidateComplete(all bool, source *readsource.Metadata) error {
	return inventory.ValidateAll(all, source)
}
func (workbookReadProvider) RefreshError(operation, environment, site string, err error) error {
	return inventory.RefreshError(operation, environment, site, err)
}
func (p workbookReadProvider) Now() time.Time { return p.commands.runtime.now() }
