package cache

import (
	"testing"
	"time"
)

func TestNameFilterPagesWithoutImplyingSingleResource(t *testing.T) {
	store := NewStore(t.TempDir(), time.Now)
	_, err := store.ReplaceResourceScope(t.Context(), ResourceScopeReplacement{
		Environment: "test", Kind: "workbook", Source: "fixture", GeneratedAt: time.Now(),
		Entries: []ResourceEntry{{LUID: "wb-1", Name: "Finance", ProjectLUID: "p-1"}, {LUID: "wb-2", Name: "Finance", ProjectLUID: "p-2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	query := ResourceQuery{Environment: "test", Kind: "workbook", Name: "Finance", Limit: 1}
	first, err := store.ReadResources(t.Context(), query)
	if err != nil || first.Total != 2 || len(first.Entries) != 1 || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	query.Cursor = first.NextCursor
	second, err := store.ReadResources(t.Context(), query)
	if err != nil || len(second.Entries) != 1 || second.Entries[0].LUID == first.Entries[0].LUID || second.NextCursor != "" {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	query.Cursor, query.Name = "", "Missing"
	empty, err := store.ReadResources(t.Context(), query)
	if err != nil || empty.Total != 0 || len(empty.Entries) != 0 || empty.Coverage != "complete" {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}
	query.ExactlyOne = true
	if _, err := store.ReadResources(t.Context(), query); err == nil {
		t.Fatal("missing exact inspection accepted")
	}
	query.Name = "Finance"
	if _, err := store.ReadResources(t.Context(), query); err == nil {
		t.Fatal("ambiguous exact inspection accepted")
	}
}
