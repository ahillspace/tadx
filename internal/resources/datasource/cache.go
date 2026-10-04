package datasource

import (
	"context"
	"encoding/json"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

type CacheSupport struct {
	ReadError          func(string, string, string, error) error
	UnsupportedFilters func(string, string, string) error
	ReadSource         func(cache.ResourceResult) *readsource.Metadata
	RecordSource       func(cache.ResourceResult, cache.ResourceEntry) *readsource.Metadata
}

type CachedListPort struct {
	store             *cache.Store
	environment, site string
	support           CacheSupport
	source            *readsource.Metadata
}

func NewCachedListPort(store *cache.Store, environment, site string, support CacheSupport) *CachedListPort {
	return &CachedListPort{store: store, environment: environment, site: site, support: support}
}
func (p *CachedListPort) Source() *readsource.Metadata { return p.source }
func (p *CachedListPort) ListDatasources(ctx context.Context, input datasource.ListPageRequest) (datasource.ListPage, error) {
	if input.OwnerName != "" || input.Type != "" || input.Tag != "" || input.UpdatedAfter != "" || input.UpdatedBefore != "" {
		return datasource.ListPage{}, p.support.UnsupportedFilters("datasource.list", p.environment, p.site)
	}
	offset := 0
	if input.SnapshotCursor == "" {
		offset = (input.PageNumber - 1) * input.PageSize
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "datasource", Name: input.Name, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Offset: offset, Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return datasource.ListPage{}, p.support.ReadError("datasource.list", p.environment, p.site, err)
	}
	p.source = p.support.ReadSource(result)
	items := make([]datasource.Record, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) == 0 || json.Unmarshal(entry.Payload, &items[index]) != nil {
			items[index] = datasource.Record{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
		}
		items[index].ProjectPath = entry.ProjectPath
		if input.ProjectName != "" {
			items[index].ProjectName = input.ProjectName
		}
	}
	return datasource.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Datasources: items, SnapshotCursor: result.NextCursor}, nil
}

type CachedInspectPort struct {
	store             *cache.Store
	environment, site string
	support           CacheSupport
	source            *readsource.Metadata
}

func NewCachedInspectPort(store *cache.Store, environment, site string, support CacheSupport) *CachedInspectPort {
	return &CachedInspectPort{store: store, environment: environment, site: site, support: support}
}
func (p *CachedInspectPort) Source() *readsource.Metadata { return p.source }
func (p *CachedInspectPort) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasource.Record, error) {
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "datasource", LUID: string(selector.LUID), Name: selector.Name, ProjectLUID: string(selector.ProjectLUID), ProjectPath: selector.ProjectPath, Limit: 2, ExactlyOne: true})
	if err != nil {
		return datasource.Record{}, p.support.ReadError("datasource.inspect", p.environment, p.site, err)
	}
	entry := result.Entries[0]
	p.source = p.support.RecordSource(result, entry)
	var item datasource.Record
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return datasource.Record{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}
