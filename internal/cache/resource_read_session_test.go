package cache

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestResourceReadSessionKeepsSnapshotWhileRefreshPublishesAndRejectsOldCursorInNextCommand(t *testing.T) {
	store := NewStore(t.TempDir(), nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	replacement := ResourceScopeReplacement{Environment: "fixture", Site: "fixture", Kind: "workbook", Source: "tableau-rest", GeneratedAt: time.Now(), Entries: []ResourceEntry{{LUID: "a", Name: "Alpha"}, {LUID: "b", Name: "Before"}, {LUID: "c", Name: "Charlie"}}}
	if _, err := store.ReplaceResourceScope(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	reader, err := store.OpenResourceRead(ctx, "fixture", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	query := ResourceQuery{Environment: "fixture", Site: "fixture", Kind: "workbook", Limit: 1}
	first, err := reader.ReadResources(ctx, query)
	if err != nil || len(first.Entries) != 1 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	replacement.Entries[1].Name = "Beta after refresh"
	replacement.GeneratedAt = replacement.GeneratedAt.Add(time.Minute)
	refreshed := make(chan error, 1)
	go func() {
		_, err := store.ReplaceResourceScope(ctx, replacement)
		refreshed <- err
	}()
	select {
	case err := <-refreshed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("a consistent read session blocked cache generation publication")
	}
	query.Cursor = first.NextCursor
	changedLimit := query
	changedLimit.Limit = 2
	if _, err := reader.ReadResources(ctx, changedLimit); err == nil {
		t.Fatal("read session accepted a cursor with a different page limit")
	}
	second, err := reader.ReadResources(ctx, query)
	if err != nil || len(second.Entries) != 1 || second.Entries[0].Name != "Before" || second.GenerationID != first.GenerationID || second.Total != first.Total || !second.InventoryObservedAt.Equal(first.InventoryObservedAt) {
		t.Fatalf("one read session crossed cache generations: first=%+v second=%+v error=%v", first, second, err)
	}
	if _, err := store.ReadResources(ctx, query); err == nil {
		t.Fatal("a subsequent command accepted a cursor from the replaced generation")
	} else if invalid, ok := errors.AsType[invalidCursorError](err); !ok || !invalid.InvalidCacheCursor() {
		t.Fatalf("old cursor returned unexpected error: %v", err)
	}
}

func TestResourceReadSessionRetainsPartialObservationsAndTargetIsolation(t *testing.T) {
	now := time.Now()
	store := NewStore(t.TempDir(), func() time.Time { return now })
	entries := []ResourceEntry{{Environment: "fixture", Site: "fixture", Kind: "workbook", LUID: "a", Name: "Alpha", Coverage: "summary", ObservedAt: now}, {Environment: "fixture", Site: "fixture", Kind: "workbook", LUID: "b", Name: "Before", Coverage: "detail", ObservedAt: now}}
	if err := store.UpsertResources(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	reader, err := store.OpenResourceRead(t.Context(), "fixture", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	query := ResourceQuery{Environment: "fixture", Site: "fixture", Kind: "workbook", Limit: 1}
	first, err := reader.ReadResources(t.Context(), query)
	if err != nil || first.Coverage != "partial" || first.Total != 2 {
		t.Fatalf("partial page = %+v, %v", first, err)
	}
	entries[1].Name, entries[1].ObservedAt = "Beta after update", now.Add(time.Minute)
	if err := store.UpsertResources(t.Context(), entries[1:]); err != nil {
		t.Fatal(err)
	}
	query.Offset = 1
	second, err := reader.ReadResources(t.Context(), query)
	if err != nil || len(second.Entries) != 1 || second.Entries[0].Name != "Before" || !second.InventoryObservedAt.Equal(first.InventoryObservedAt) {
		t.Fatalf("partial observations changed inside one snapshot: %+v, %v", second, err)
	}
	for _, other := range []ResourceQuery{{Environment: "other", Site: "fixture", Kind: "workbook", Limit: 1}, {Environment: "fixture", Site: "other", Kind: "workbook", Limit: 1}} {
		if _, err := reader.ReadResources(t.Context(), other); err == nil {
			t.Fatalf("read session accepted another target: %+v", other)
		}
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadResources(t.Context(), query); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("closed read session remained usable: %v", err)
	}
}

func TestResourceReadSessionPreservesImmutablePartialInventoryContinuation(t *testing.T) {
	store := NewStore(t.TempDir(), nil)
	entries := []ResourceEntry{{LUID: "a", Name: "Alpha"}, {LUID: "b", Name: "Beta"}}
	for index := range entries {
		entries[index].Environment, entries[index].Site, entries[index].Kind = "fixture", "fixture", "workbook"
		entries[index].ObservedAt, entries[index].Coverage = time.Now(), "summary"
	}
	id, err := store.SavePartialInventory(t.Context(), ResourceScopeReplacement{Environment: "fixture", Site: "fixture", Kind: "workbook", Source: "tableau-rest", GeneratedAt: time.Now(), Entries: entries}, "bounded inventory")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := store.OpenResourceRead(t.Context(), "fixture", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	query := ResourceQuery{Environment: "fixture", Site: "fixture", Kind: "workbook", Limit: 1}
	query.Cursor = PartialInventoryCursor(id, query)
	first, err := reader.ReadResources(t.Context(), query)
	if err != nil || len(first.Entries) != 1 || first.NextCursor == "" || first.InventoryWarning != "bounded inventory" {
		t.Fatalf("first immutable inventory page = %+v, %v", first, err)
	}
	query.Cursor = first.NextCursor
	second, err := reader.ReadResources(t.Context(), query)
	if err != nil || len(second.Entries) != 1 || second.Entries[0].LUID != "b" || second.NextCursor != "" {
		t.Fatalf("second immutable inventory page = %+v, %v", second, err)
	}
	oneShot, err := store.ReadResources(t.Context(), query)
	if err != nil || !reflect.DeepEqual(oneShot, second) {
		t.Fatalf("one-shot and session readers differ: %+v / %+v, %v", oneShot, second, err)
	}
}
