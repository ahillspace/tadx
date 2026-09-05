package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestReadThroughResourcesRemainPartialAndPreserveUnrelatedRecords(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	entries := []ResourceEntry{
		{Environment: "production", Site: "marketing", Kind: "workbook", LUID: "wb-1", Name: "Finance", Payload: []byte(`{"luid":"wb-1","name":"Finance"}`), Coverage: "detail", ObservedAt: now},
		{Environment: "production", Site: "marketing", Kind: "workbook", LUID: "wb-2", Name: "Sales", Payload: []byte(`{"luid":"wb-2","name":"Sales"}`), Coverage: "summary", ObservedAt: now},
	}
	if err := store.UpsertResources(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertResources(context.Background(), []ResourceEntry{{Environment: "production", Site: "marketing", Kind: "workbook", LUID: "wb-1", Name: "Finance Updated", Payload: []byte(`{"luid":"wb-1","name":"Finance Updated"}`), Coverage: "summary", ObservedAt: now.Add(time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	result, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage != "partial" || result.Total != 2 || len(result.Entries) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.Entries[0].Coverage != "detail" || result.Entries[0].Name != "Finance Updated" {
		t.Fatalf("updated entry = %#v", result.Entries[0])
	}
	if string(result.Entries[0].Payload) != `{"luid":"wb-1","name":"Finance"}` || !result.Entries[0].ObservedAt.Equal(now) {
		t.Fatalf("detail projection was replaced by summary data: %#v", result.Entries[0])
	}
}

func TestPublishedGenerationSeedsCompleteResourceCoverage(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	_, err := store.Replace(context.Background(), Generation{ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Source: "tableau-rest", Scopes: []string{"workbooks"}, Records: []Record{{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Operations"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage != "complete" || result.GenerationID != "generation-1" || result.Total != 1 || result.Entries[0].ProjectPath != "Operations" {
		t.Fatalf("result = %#v", result)
	}
}

func TestResourceReadDistinguishesUninitializedScopeAndMissingRecord(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	_, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 25})
	var uninitialized interface{ CatalogUninitialized() bool }
	if !errors.As(err, &uninitialized) {
		t.Fatalf("uninitialized error = %v", err)
	}
	if _, err := store.Replace(context.Background(), Generation{ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Source: "tableau-rest", Scopes: []string{"projects"}, Records: []Record{{LUID: "p-1", Kind: "project", Name: "Operations"}}}); err != nil {
		t.Fatal(err)
	}
	_, err = store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 25})
	var unavailable interface{ CatalogScopeUnavailable() bool }
	if !errors.As(err, &unavailable) {
		t.Fatalf("scope error = %v", err)
	}
	_, err = store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "project", LUID: "missing", Limit: 2})
	var missing interface{ CatalogResourceNotFound() bool }
	if !errors.As(err, &missing) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestReplaceResourceScopeAtomicallyPreservesOtherKindsAndDetailCoverage(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	seed := []ResourceEntry{
		{Environment: "production", Site: "marketing", Kind: "workbook", LUID: "wb-keep", Name: "Old name", Payload: []byte(`{"description":"retained"}`), Coverage: "detail", ObservedAt: now.Add(-time.Hour)},
		{Environment: "production", Site: "marketing", Kind: "workbook", LUID: "wb-stale", Name: "Removed", Coverage: "summary", ObservedAt: now.Add(-time.Hour)},
		{Environment: "production", Site: "marketing", Kind: "datasource", LUID: "ds-keep", Name: "Datasource", Coverage: "summary", ObservedAt: now.Add(-time.Hour)},
	}
	if err := store.UpsertResources(context.Background(), seed); err != nil {
		t.Fatal(err)
	}

	replaced, err := store.ReplaceResourceScope(context.Background(), ResourceScopeReplacement{
		Environment: "production", Site: "marketing", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now,
		Entries: []ResourceEntry{
			{LUID: "wb-new", Name: "New", ProjectPath: "Sales"},
			{LUID: "wb-keep", Name: "Current name", ProjectPath: "Operations", Owner: "owner-new", Payload: []byte(`{"luid":"wb-keep","name":"Current name","project_path":"Operations","owner_luid":"owner-new"}`)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.GenerationID == "" || replaced.RecordCount != 2 {
		t.Fatalf("replacement = %#v", replaced)
	}
	workbooks, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if workbooks.Coverage != "complete" || workbooks.GenerationID != replaced.GenerationID || !workbooks.GeneratedAt.Equal(now) || workbooks.Total != 2 {
		t.Fatalf("workbook snapshot = %#v", workbooks)
	}
	var retained map[string]any
	if err := json.Unmarshal(workbooks.Entries[0].Payload, &retained); err != nil {
		t.Fatal(err)
	}
	if workbooks.Entries[0].LUID != "wb-keep" || workbooks.Entries[0].Name != "Current name" || workbooks.Entries[0].Owner != "owner-new" || workbooks.Entries[0].Coverage != "detail" || retained["description"] != "retained" || retained["name"] != "Current name" || retained["owner_luid"] != "owner-new" {
		t.Fatalf("retained detail = %#v", workbooks.Entries[0])
	}
	datasources, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "datasource", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if datasources.Total != 1 || datasources.Entries[0].LUID != "ds-keep" || datasources.Coverage != "partial" {
		t.Fatalf("unrelated scope = %#v", datasources)
	}
}

func TestResourceScopeCursorIsBoundToCompleteSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	replace := func(names ...string) ReplaceResult {
		t.Helper()
		entries := make([]ResourceEntry, len(names))
		for index, name := range names {
			entries[index] = ResourceEntry{LUID: fmt.Sprintf("wb-%d", index+1), Name: name}
		}
		result, err := store.ReplaceResourceScope(context.Background(), ResourceScopeReplacement{Environment: "production", Site: "marketing", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now, Entries: entries})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	firstGeneration := replace("Alpha", "Beta", "Gamma")
	first, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if first.GenerationID != firstGeneration.GenerationID || len(first.Entries) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	second, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 1 || second.Entries[0].Name != "Gamma" || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}

	replace("Alpha", "Beta changed", "Gamma")
	_, err = store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 2, Cursor: first.NextCursor})
	var invalid interface{ InvalidCatalogCursor() bool }
	if !errors.As(err, &invalid) {
		t.Fatalf("stale cursor error = %v", err)
	}
}

func TestReplaceResourceScopeRejectsPartialOrDuplicateInputWithoutChangingCurrentSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	initial, err := store.ReplaceResourceScope(context.Background(), ResourceScopeReplacement{
		Environment: "production", Kind: "group", Source: "tableau-rest", GeneratedAt: now,
		Entries: []ResourceEntry{{LUID: "group-1", Name: "All Users"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceResourceScope(context.Background(), ResourceScopeReplacement{
		Environment: "production", Kind: "group", Source: "tableau-rest", GeneratedAt: now.Add(time.Minute),
		Entries: []ResourceEntry{{LUID: "group-2", Name: "One"}, {LUID: "group-2", Name: "Two"}},
	})
	if err == nil {
		t.Fatal("duplicate authoritative identity was accepted")
	}
	current, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Kind: "group", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if current.GenerationID != initial.GenerationID || current.Total != 1 || current.Entries[0].LUID != "group-1" {
		t.Fatalf("current snapshot changed after rejected replacement: %#v", current)
	}
}

func TestFullAndTargetedResourceSnapshotsSupersedeOnlyTheirIntendedScopes(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	fullOne, err := store.Replace(context.Background(), Generation{
		ID: "full-one", Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Source: "tableau-rest",
		Scopes: []string{"workbooks", "datasources"}, Records: []Record{
			{LUID: "wb-1", Kind: "workbook", Name: "Full workbook"},
			{LUID: "ds-1", Kind: "datasource", Name: "Full datasource"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	targeted, err := store.ReplaceResourceScope(context.Background(), ResourceScopeReplacement{
		Environment: "production", Site: "marketing", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now.Add(time.Minute),
		Entries: []ResourceEntry{{LUID: "wb-2", Name: "Targeted workbook"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	workbooks, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	datasources, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "datasource", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if workbooks.GenerationID != targeted.GenerationID || workbooks.Entries[0].LUID != "wb-2" {
		t.Fatalf("targeted workbook snapshot = %#v", workbooks)
	}
	if datasources.GenerationID != fullOne.GenerationID || datasources.Entries[0].LUID != "ds-1" {
		t.Fatalf("preserved datasource snapshot = %#v", datasources)
	}

	fullTwo, err := store.Replace(context.Background(), Generation{
		ID: "full-two", Environment: "production", Site: "marketing", GeneratedAt: now.Add(2 * time.Minute), Complete: true, Source: "tableau-rest",
		Scopes: []string{"workbooks", "datasources"}, Records: []Record{
			{LUID: "wb-3", Kind: "workbook", Name: "Newest full workbook"},
			{LUID: "ds-2", Kind: "datasource", Name: "Newest full datasource"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	workbooks, err = store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if workbooks.GenerationID != fullTwo.GenerationID || workbooks.Entries[0].LUID != "wb-3" {
		t.Fatalf("full refresh did not supersede targeted snapshot: %#v", workbooks)
	}
}

func TestReadThroughUpsertInvalidatesCompleteScopeSnapshotAndItsCursor(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	_, err := store.ReplaceResourceScope(context.Background(), ResourceScopeReplacement{
		Environment: "production", Kind: "user", Source: "tableau-rest", GeneratedAt: now,
		Entries: []ResourceEntry{{LUID: "user-1", Name: "Alpha"}, {LUID: "user-2", Name: "Beta"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Kind: "user", Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("complete page = %#v; error = %v", page, err)
	}
	if err := store.UpsertResources(context.Background(), []ResourceEntry{{Environment: "production", Kind: "user", LUID: "user-1", Name: "Alpha updated", Coverage: "detail", ObservedAt: now.Add(time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	_, err = store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Kind: "user", Limit: 1, Cursor: page.NextCursor})
	var invalid interface{ InvalidCatalogCursor() bool }
	if !errors.As(err, &invalid) {
		t.Fatalf("invalidated cursor error = %v", err)
	}
	current, err := store.ReadResources(context.Background(), ResourceQuery{Environment: "production", Kind: "user", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if current.Coverage != "partial" || current.GenerationID != "" || current.Entries[0].Name != "Alpha updated" {
		t.Fatalf("read-through result = %#v", current)
	}
}
