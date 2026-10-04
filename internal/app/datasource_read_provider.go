package app

import (
	"context"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
)

type datasourceReadProvider struct {
	commands *remoteContentCommands
	projects *resourceproject.DiscoveryPaths
	resourcedatasource.ListFilterPort
}

func (p *datasourceReadProvider) CacheTarget(alias string) (datasource.ReadTarget, error) {
	environment, site, err := p.commands.resolveCacheTarget(alias)
	return datasource.ReadTarget{Environment: environment, Site: site}, err
}

func (p *datasourceReadProvider) CachedList(target datasource.ReadTarget) datasource.CachedListReader {
	return resourcedatasource.NewCachedListPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (p *datasourceReadProvider) CachedInspect(target datasource.ReadTarget) datasource.CachedInspectResolver {
	return resourcedatasource.NewCachedInspectPort(p.commands.cacheStore(target.Environment), target.Environment, target.Site, p.cacheSupport())
}

func (p *datasourceReadProvider) OpenDatasourceRead(ctx context.Context, alias, site, operation string) (datasource.ReadSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return datasource.ReadSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	target := datasource.ReadTarget{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}
	if p.projects == nil {
		p.projects = resourceproject.NewDiscoveryPaths(connection.projects)
	}
	ports := resourcedatasource.ReadPorts{Adapter: connection.datasources, Projects: p.projects}
	collection := resourcedatasource.InventoryPorts{
		Executor:    connection.inventory,
		Store:       func() *cache.Store { return p.commands.cacheStore(target.Environment) },
		Environment: target.Environment, Site: target.Site,
		MaxConcurrency: connection.environment.CacheMaxConcurrency, Now: p.commands.runtime.now,
	}
	return datasource.ReadSession{ReadTarget: target, Reader: ports, Resolver: ports, Upstream: resourcedatasource.UpstreamPort{Client: connection.metadataAssets}, Inventory: collection}, nil
}

func (p *datasourceReadProvider) cacheSupport() resourcedatasource.CacheSupport {
	return resourcedatasource.CacheSupport{ReadError: inventory.CacheReadError, UnsupportedFilters: inventory.UnsupportedCacheFilters, ReadSource: inventory.CacheReadSource, RecordSource: inventory.CacheRecordSource}
}

func (p *datasourceReadProvider) Now() time.Time { return p.commands.runtime.now() }
