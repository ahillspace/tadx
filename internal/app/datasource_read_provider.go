package app

import (
	"context"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type datasourceReadProvider struct {
	commands *remoteContentCommands
	projects *resourceproject.DiscoveryPaths
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

func (*datasourceReadProvider) LegacyInventoryCursor(cursor string) bool {
	return legacyInventorySnapshot(cursor)
}

func (*datasourceReadProvider) ListFilter(input datasource.ListInput) (string, error) {
	return tableaudatasource.ListFilter(tableaudatasource.ListRequest{
		Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID,
		ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
	})
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
	return resourcedatasource.CacheSupport{ReadError: cacheReadError, UnsupportedFilters: unsupportedCacheFilters, ReadSource: cacheReadSource, RecordSource: cacheRecordSource}
}

func (*datasourceReadProvider) ValidateComplete(all bool, source *readsource.Metadata) error {
	return inventory.ValidateAll(all, source)
}

func (*datasourceReadProvider) RefreshError(operation, environment, site string, err error) error {
	return inventory.RefreshError(operation, environment, site, err)
}

func (p *datasourceReadProvider) Now() time.Time { return p.commands.runtime.now() }
