package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

type InventoryPorts struct {
	Executor          tableaucache.Executor
	Store             func() *cache.Store
	Environment, Site string
	MaxConcurrency    int
	Now               func() time.Time
}

func (p InventoryPorts) CollectDatasources(ctx context.Context, filter string, observedAt time.Time) (datasource.CollectedList, error) {
	collected, err := inventory.Collect(ctx, p.Executor, p.Store(), tableaucache.ScopeDatasources, p.Environment, p.Site, observedAt, inventory.Options{MaxConcurrency: p.MaxConcurrency, Filter: filter})
	if err != nil {
		return datasource.CollectedList{}, err
	}
	source, help := collected.SourceAndHelp(observedAt, p.Now)
	requestID := collected.FinalRequestID()
	return datasource.CollectedList{Reader: inventoryPages{entries: collected.Entries, requestID: requestID}, Source: source, RequestID: requestID, Help: help}, nil
}

type inventoryPages struct {
	entries   []cache.ResourceEntry
	requestID string
}

func (p inventoryPages) ListDatasources(_ context.Context, input datasource.ListPageRequest) (datasource.ListPage, error) {
	start := min((input.PageNumber-1)*input.PageSize, len(p.entries))
	end := min(start+input.PageSize, len(p.entries))
	items := make([]datasource.Record, end-start)
	for index, entry := range p.entries[start:end] {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return datasource.ListPage{}, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return datasource.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(p.entries), Datasources: items, RequestID: p.requestID}, nil
}

func (p InventoryPorts) PublishDatasourceInspect(_ context.Context, output datasource.InspectOutput, observedAt time.Time) {
	payload, err := json.Marshal(output.Datasource)
	if err != nil {
		return
	}
	inventory.PublishDetail(p.Store(), cache.ResourceEntry{Environment: output.Environment, Site: output.Site, Kind: "datasource", LUID: output.Datasource.LUID, Name: output.Datasource.Name, ProjectLUID: output.Datasource.ProjectLUID, ProjectPath: output.Datasource.ProjectPath, Owner: output.Datasource.OwnerLUID, Payload: payload, Coverage: "detail", ObservedAt: observedAt})
}
