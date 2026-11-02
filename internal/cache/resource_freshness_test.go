package cache

import (
	"testing"
	"time"
)

func TestReadThroughFreshnessUsesReturnedEvidence(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	if err := store.UpsertResources(t.Context(), []ResourceEntry{
		{Environment: "dev", Kind: "workbook", LUID: "old", Name: "A old", Coverage: "summary", ObservedAt: old},
		{Environment: "dev", Kind: "workbook", LUID: "new", Name: "Z new", Coverage: "summary", ObservedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, id string
		limit    int
		stale    bool
		observed time.Time
	}{
		{"old page excludes fresh row", "", 1, true, old},
		{"mixed page", "", 2, true, now},
		{"old exact read", "old", 1, true, old},
		{"fresh exact read", "new", 1, false, now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.ReadResources(t.Context(), ResourceQuery{Environment: "dev", Kind: "workbook", LUID: tc.id, Limit: tc.limit})
			if err != nil {
				t.Fatal(err)
			}
			inventoryObserved := now
			if tc.id == "old" {
				inventoryObserved = old
			}
			if got.Stale != tc.stale || !got.NewestObserved.Equal(tc.observed) || got.Coverage != "partial" || !got.InventoryObservedAt.Equal(inventoryObserved) {
				t.Fatalf("stale=%t newest=%s coverage=%s", got.Stale, got.NewestObserved, got.Coverage)
			}
		})
	}
}

func TestCompleteResourceFreshnessUsesAuthoritativeGeneration(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	if _, err := store.ReplaceResourceScope(t.Context(), ResourceScopeReplacement{
		Environment: "dev", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now.Add(-48 * time.Hour),
		Entries: []ResourceEntry{{LUID: "old", Name: "A old"}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadResources(t.Context(), ResourceQuery{Environment: "dev", Kind: "workbook", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stale || got.Coverage != "complete" || !got.GeneratedAt.Equal(now.Add(-48*time.Hour)) {
		t.Fatalf("generation evidence was not preserved: %+v", got)
	}
}
