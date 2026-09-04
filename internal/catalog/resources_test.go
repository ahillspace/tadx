package catalog

import (
	"context"
	"errors"
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
