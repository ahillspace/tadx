package cache

import (
	"context"
	"testing"
	"time"
)

func TestReadResourcesFiltersByAuthoritativeProjectLUID(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store := NewStore(t.TempDir(), func() time.Time { return now })
	_, err := store.ReplaceResourceScope(ctx, ResourceScopeReplacement{
		Environment: "test", Site: "site", Kind: "workbook", Source: "test", GeneratedAt: now,
		Entries: []ResourceEntry{
			{LUID: "wb-1", Name: "Sales", ProjectLUID: "project-1", ObservedAt: now, Coverage: "summary"},
			{LUID: "wb-2", Name: "Sales", ProjectLUID: "project-2", ObservedAt: now, Coverage: "summary"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ReadResources(ctx, ResourceQuery{Environment: "test", Site: "site", Kind: "workbook", ProjectLUID: "project-2", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Entries) != 1 || result.Entries[0].LUID != "wb-2" {
		t.Fatalf("result = %#v", result)
	}
}
