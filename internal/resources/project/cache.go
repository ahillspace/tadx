package project

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

// CacheSupport binds shared cache error and coverage rules to project readers.
type CacheSupport struct {
	ReadError          func(string, string, string, error) error
	UnsupportedFilters func(string, string, string) error
	ReadSource         func(cache.ResourceResult) *readsource.Metadata
	RecordSource       func(cache.ResourceResult, cache.ResourceEntry) *readsource.Metadata
}

// CachedListPort translates indexed project entries into the list action port.
type CachedListPort struct {
	store       *cache.Store
	environment string
	site        string
	support     CacheSupport
	source      *readsource.Metadata
}

func NewCachedListPort(store *cache.Store, environment, site string, support CacheSupport) *CachedListPort {
	return &CachedListPort{store: store, environment: environment, site: site, support: support}
}

func (p *CachedListPort) Source() *readsource.Metadata { return p.source }

func (p *CachedListPort) ListProjects(ctx context.Context, input projectops.ListPageRequest) (projectops.ListPage, error) {
	if input.ParentLUID != "" || input.OwnerName != "" || input.TopLevel != nil {
		return projectops.ListPage{}, p.support.UnsupportedFilters("project.list", p.environment, p.site)
	}
	offset := 0
	if input.SnapshotCursor == "" {
		offset = (input.PageNumber - 1) * input.PageSize
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "project", Name: input.Name, Offset: offset, Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return projectops.ListPage{}, p.support.ReadError("project.list", p.environment, p.site, err)
	}
	p.source = p.support.ReadSource(result)
	items := make([]projectops.ListProject, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = projectops.ListProject{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
	}
	return projectops.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Projects: items, SnapshotCursor: result.NextCursor}, nil
}

// CachedInspectPort translates one exact cached project into the inspect port.
type CachedInspectPort struct {
	store       *cache.Store
	environment string
	site        string
	support     CacheSupport
	source      *readsource.Metadata
}

func NewCachedInspectPort(store *cache.Store, environment, site string, support CacheSupport) *CachedInspectPort {
	return &CachedInspectPort{store: store, environment: environment, site: site, support: support}
}

func (p *CachedInspectPort) Source() *readsource.Metadata { return p.source }

func (p *CachedInspectPort) ResolveProject(ctx context.Context, selector identity.Selector) (projectops.InspectProject, error) {
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "project", LUID: string(selector.LUID), ProjectPath: selector.ProjectPath, Limit: 2, ExactlyOne: true})
	if err != nil {
		return projectops.InspectProject{}, p.support.ReadError("project.inspect", p.environment, p.site, err)
	}
	entry := result.Entries[0]
	p.source = p.support.RecordSource(result, entry)
	var item projectops.InspectProject
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		item.LUID, item.Name, item.Path, item.OwnerLUID = entry.LUID, entry.Name, entry.ProjectPath, entry.Owner
		return item, nil
	}
	return projectops.InspectProject{LUID: entry.LUID, Name: entry.Name, Path: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}

// InventoryListPort translates complete live inventory entries into bounded pages.
type InventoryListPort struct {
	Entries           []cache.ResourceEntry
	RequestID         string
	AllowContinuation bool
}

func (p InventoryListPort) ListProjects(_ context.Context, input projectops.ListPageRequest) (projectops.ListPage, error) {
	start := min((input.PageNumber-1)*input.PageSize, len(p.Entries))
	end := min(start+input.PageSize, len(p.Entries))
	items := make([]projectops.ListProject, end-start)
	for index, entry := range p.Entries[start:end] {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return projectops.ListPage{}, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return projectops.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(p.Entries), Projects: items, RequestID: p.RequestID, SuppressContinuation: !p.AllowContinuation}, nil
}

// InspectEntry encodes one confirmed live project detail for cache publication.
func InspectEntry(output projectops.InspectOutput, observedAt time.Time) (cache.ResourceEntry, error) {
	encoded, err := json.Marshal(output.Project)
	if err != nil {
		return cache.ResourceEntry{}, err
	}
	return cache.ResourceEntry{
		Environment: output.Environment, Site: output.Site, Kind: "project",
		LUID: output.Project.LUID, Name: output.Project.Name, ProjectPath: output.Project.Path,
		Owner: output.Project.OwnerLUID, Payload: encoded, Coverage: "detail", ObservedAt: observedAt,
	}, nil
}
