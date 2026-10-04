package workbook

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type ReadPorts struct{ Adapter *Adapter }

// ListFilterPort validates and encodes a native workbook list selection before authentication.
type ListFilterPort struct{}

func (ListFilterPort) ListFilter(input workbook.ListInput) (string, error) {
	return tableauworkbook.ListFilter(tableauworkbook.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
}

func (p ReadPorts) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbook.Record, error) {
	return p.Adapter.ResolveWorkbook(ctx, selector)
}

func (p ReadPorts) ListWorkbooks(ctx context.Context, input workbook.ListPageRequest) (workbook.ListPage, error) {
	page, err := p.Adapter.ListWorkbooks(ctx, tableauworkbook.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
	return workbookPage(page, err)
}

func workbookPage(page Page, err error) (workbook.ListPage, error) {
	items := make([]workbook.Record, len(page.Items))
	for index, item := range page.Items {
		items[index] = item
		items[index].Tags = append([]string(nil), item.Tags...)
	}
	return workbook.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Workbooks: items, RequestID: page.RequestID}, err
}

type InventoryPorts struct {
	Adapter           *Adapter
	Executor          tableaucache.Executor
	Store             func() *cache.Store
	Environment, Site string
	MaxConcurrency    int
	Now               func() time.Time
}

func (p InventoryPorts) CollectProjectWorkbooks(ctx context.Context, input workbook.ListInput) (workbook.ListReader, error) {
	page, err := p.Adapter.CollectProjectWorkbooks(ctx, tableauworkbook.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
	if err != nil {
		return nil, err
	}
	return projectWorkbookPages{snapshot: page}, nil
}

type projectWorkbookPages struct{ snapshot Page }

func (p projectWorkbookPages) ListWorkbooks(_ context.Context, input workbook.ListPageRequest) (workbook.ListPage, error) {
	if input.PageNumber < 1 || input.PageSize < 1 {
		return workbook.ListPage{}, fmt.Errorf("invalid workbook snapshot page")
	}
	start := (input.PageNumber - 1) * input.PageSize
	if start > len(p.snapshot.Items) {
		return workbook.ListPage{}, fmt.Errorf("workbook snapshot page exceeds the collected total")
	}
	end := min(start+input.PageSize, len(p.snapshot.Items))
	return workbookPage(Page{Number: input.PageNumber, Size: input.PageSize, Total: p.snapshot.Total, Items: p.snapshot.Items[start:end], RequestID: p.snapshot.RequestID}, nil)
}

func (p InventoryPorts) CollectWorkbooks(ctx context.Context, filter string, observedAt time.Time) (workbook.CollectedList, error) {
	collected, err := inventory.Collect(ctx, p.Executor, p.Store(), tableaucache.ScopeWorkbooks, p.Environment, p.Site, observedAt, inventory.Options{MaxConcurrency: p.MaxConcurrency, Filter: filter})
	if err != nil {
		return workbook.CollectedList{}, err
	}
	source, help := collected.SourceAndHelp(observedAt, p.Now)
	requestID := collected.FinalRequestID()
	return workbook.CollectedList{Reader: workbookInventoryPages{entries: collected.Entries, requestID: requestID}, Source: source, RequestID: requestID, Help: help}, nil
}

type workbookInventoryPages struct {
	entries   []cache.ResourceEntry
	requestID string
}

func (p workbookInventoryPages) ListWorkbooks(_ context.Context, input workbook.ListPageRequest) (workbook.ListPage, error) {
	start := min((input.PageNumber-1)*input.PageSize, len(p.entries))
	end := min(start+input.PageSize, len(p.entries))
	items := make([]workbook.Record, end-start)
	for index, entry := range p.entries[start:end] {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return workbook.ListPage{}, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return workbook.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(p.entries), Workbooks: items, RequestID: p.requestID}, nil
}

func (p InventoryPorts) PublishWorkbookInspect(_ context.Context, output workbook.InspectOutput, observedAt time.Time) {
	payload, err := json.Marshal(output.Workbook)
	if err != nil {
		return
	}
	inventory.PublishDetail(p.Store(), cache.ResourceEntry{Environment: output.Environment, Site: output.Site, Kind: "workbook", LUID: output.Workbook.LUID, Name: output.Workbook.Name, ProjectLUID: output.Workbook.ProjectLUID, ProjectPath: output.Workbook.ProjectPath, Owner: output.Workbook.OwnerLUID, Payload: payload, Coverage: "detail", ObservedAt: observedAt})
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

func (p *CachedListPort) ListWorkbooks(ctx context.Context, input workbook.ListPageRequest) (workbook.ListPage, error) {
	if input.OwnerName != "" || input.ProjectName != "" || input.Tag != "" {
		return workbook.ListPage{}, p.support.UnsupportedFilters("workbook.list", p.environment, p.site)
	}
	offset := 0
	if input.SnapshotCursor == "" {
		offset = (input.PageNumber - 1) * input.PageSize
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "workbook", Name: input.Name, ProjectLUID: input.ProjectLUID, Offset: offset, Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return workbook.ListPage{}, p.support.ReadError("workbook.list", p.environment, p.site, err)
	}
	p.source = p.support.ReadSource(result)
	items := make([]workbook.Record, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = workbook.Record{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}
	}
	return workbook.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Workbooks: items, SnapshotCursor: result.NextCursor}, nil
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

func (p *CachedInspectPort) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbook.Record, error) {
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "workbook", LUID: string(selector.LUID), Name: selector.Name, ProjectLUID: string(selector.ProjectLUID), ProjectPath: selector.ProjectPath, Limit: 2, ExactlyOne: true})
	if err != nil {
		return workbook.Record{}, p.support.ReadError("workbook.inspect", p.environment, p.site, err)
	}
	entry := result.Entries[0]
	p.source = p.support.RecordSource(result, entry)
	var item workbook.Record
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return workbook.Record{LUID: entry.LUID, Name: entry.Name, ProjectPath: entry.ProjectPath, OwnerLUID: entry.Owner}, nil
}
