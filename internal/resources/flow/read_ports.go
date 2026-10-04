package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

type ReadPorts struct{ Adapter *Adapter }

func (p ReadPorts) ResolveFlow(ctx context.Context, selector identity.Selector) (flow.Record, error) {
	return p.Adapter.ResolveFlow(ctx, selector)
}

func (p ReadPorts) ListFlows(ctx context.Context, input flow.ListPageRequest) (flow.ListPage, error) {
	page, err := p.Adapter.ListFlows(ctx, tableauflow.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName})
	items := make([]flow.Record, len(page.Items))
	for index, item := range page.Items {
		items[index] = flow.Record{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: item.ProjectPath, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...)}
	}
	return flow.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Flows: items, RequestID: page.RequestID}, err
}

type InventoryPorts struct {
	Executor          tableaucache.Executor
	Store             func() *cache.Store
	Environment, Site string
	MaxConcurrency    int
	Now               func() time.Time
}

func (p InventoryPorts) CollectFlows(ctx context.Context, filter string, observedAt time.Time) (flow.CollectedList, error) {
	collected, err := inventory.Collect(ctx, p.Executor, p.Store(), tableaucache.ScopeFlows, p.Environment, p.Site, observedAt, inventory.Options{MaxConcurrency: p.MaxConcurrency, Filter: filter})
	if err != nil {
		return flow.CollectedList{}, err
	}
	source, help := collected.SourceAndHelp(observedAt, p.Now)
	requestID := collected.FinalRequestID()
	return flow.CollectedList{Reader: flowInventoryPages{entries: collected.Entries, requestID: requestID}, Source: source, RequestID: requestID, Help: help}, nil
}

type flowInventoryPages struct {
	entries   []cache.ResourceEntry
	requestID string
}

func (p flowInventoryPages) ListFlows(_ context.Context, input flow.ListPageRequest) (flow.ListPage, error) {
	start := min((input.PageNumber-1)*input.PageSize, len(p.entries))
	end := min(start+input.PageSize, len(p.entries))
	items := make([]flow.Record, end-start)
	for index, entry := range p.entries[start:end] {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return flow.ListPage{}, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return flow.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(p.entries), Flows: items, RequestID: p.requestID}, nil
}

func (p InventoryPorts) PublishFlowInspect(_ context.Context, output flow.InspectOutput, observedAt time.Time) {
	projection := output.CacheFlow()
	payload, err := json.Marshal(projection)
	if err != nil {
		return
	}
	inventory.PublishDetail(p.Store(), cache.ResourceEntry{Environment: output.Environment, Site: output.Site, Kind: "flow", LUID: output.Flow.LUID, Name: output.Flow.Name, ProjectLUID: projection.ProjectLUID, ProjectPath: output.Flow.ProjectPath, Owner: output.Flow.OwnerLUID, Payload: payload, Coverage: "detail", ObservedAt: observedAt})
}

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
func (p *CachedListPort) ListFlows(ctx context.Context, input flow.ListPageRequest) (flow.ListPage, error) {
	if input.OwnerName != "" || input.ProjectLUID != "" || input.ProjectName != "" {
		return flow.ListPage{}, p.support.UnsupportedFilters("flow.list", p.environment, p.site)
	}
	offset := 0
	if input.SnapshotCursor == "" {
		offset = (input.PageNumber - 1) * input.PageSize
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "flow", Name: input.Name, Offset: offset, Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return flow.ListPage{}, p.support.ReadError("flow.list", p.environment, p.site, err)
	}
	p.source = p.support.ReadSource(result)
	items := make([]flow.Record, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) == 0 || json.Unmarshal(entry.Payload, &items[index]) != nil {
			items[index] = flow.Record{LUID: entry.LUID, Name: entry.Name, OwnerLUID: entry.Owner}
		}
		items[index].ProjectPath = entry.ProjectPath
	}
	return flow.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Flows: items, SnapshotCursor: result.NextCursor}, nil
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
func (p *CachedInspectPort) ResolveFlow(ctx context.Context, selector identity.Selector) (flow.Record, error) {
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "flow", LUID: string(selector.LUID), Name: selector.Name, ProjectPath: selector.ProjectPath, ProjectLUID: string(selector.ProjectLUID), Limit: 2, ExactlyOne: true})
	if err != nil {
		return flow.Record{}, p.support.ReadError("flow.inspect", p.environment, p.site, err)
	}
	entry := result.Entries[0]
	p.source = p.support.RecordSource(result, entry)
	var item flow.Record
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return flow.Record{LUID: entry.LUID, Name: entry.Name, ProjectLUID: entry.ProjectLUID, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}
