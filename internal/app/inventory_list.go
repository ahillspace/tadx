package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	corecache "github.com/ahillspace/tadx/internal/cache"
)

type inventoryMemoryReader struct {
	allowContinuation bool
	entries           []corecache.ResourceEntry
	requestID         string
}

func (r inventoryMemoryReader) page(number, size int) []corecache.ResourceEntry {
	start := min((number-1)*size, len(r.entries))
	end := min(start+size, len(r.entries))
	return r.entries[start:end]
}

func decodeInventoryPage[T any](entries []corecache.ResourceEntry) ([]T, error) {
	items := make([]T, len(entries))
	for index, entry := range entries {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return nil, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return items, nil
}

func (r inventoryMemoryReader) ListWorkbooks(_ context.Context, input workbookops.ListPageRequest) (workbookops.ListPage, error) {
	items, err := decodeInventoryPage[workbookops.Record](r.page(input.PageNumber, input.PageSize))
	return workbookops.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(r.entries), Workbooks: items, RequestID: r.requestID, SuppressContinuation: !r.allowContinuation}, err
}

func (r inventoryMemoryReader) ListDatasources(_ context.Context, input datasourceops.ListPageRequest) (datasourceops.ListPage, error) {
	items, err := decodeInventoryPage[datasourceops.Record](r.page(input.PageNumber, input.PageSize))
	return datasourceops.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(r.entries), Datasources: items, RequestID: r.requestID, SuppressContinuation: !r.allowContinuation}, err
}

func (r inventoryMemoryReader) ListFlows(_ context.Context, input flowops.ListPageRequest) (flowops.ListPage, error) {
	items, err := decodeInventoryPage[flowops.Record](r.page(input.PageNumber, input.PageSize))
	return flowops.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(r.entries), Flows: items, RequestID: r.requestID, SuppressContinuation: !r.allowContinuation}, err
}

// Legacy process-boundary cursors can still target a previously stored snapshot.
func legacyInventorySnapshot(value string) bool {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	for _, key := range []string{"c", "Snapshot"} {
		var token string
		if json.Unmarshal(fields[key], &token) == nil && token != "" {
			return true
		}
	}
	return false
}
