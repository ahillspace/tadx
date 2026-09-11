package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProjectNameFilterUsesImplicitProjectsAndBindsContinuation(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store := NewStore(t.TempDir(), func() time.Time { return now })
	// Projects are a complete dependency, not a requested searchable scope.
	_, err := store.Replace(ctx, Generation{ID: "implicit-projects", Environment: "test", Source: "test", GeneratedAt: now, Complete: true, Scopes: []string{"datasources"}, DependencyScopes: []string{"projects"}, Records: []Record{{LUID: "p", Kind: "project", Name: "Reports", ProjectPath: "Ops/Reports"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceResourceScope(ctx, ResourceScopeReplacement{Environment: "test", Kind: "datasource", Source: "test", GeneratedAt: now, Entries: []ResourceEntry{{LUID: "a", Name: "A", ProjectLUID: "p", Payload: []byte(`{"project_luid":"p"}`)}, {LUID: "b", Name: "B", ProjectLUID: "p", Payload: []byte(`{"project_luid":"p"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	query := ResourceQuery{Environment: "test", Kind: "datasource", ProjectName: "Reports", Limit: 1}
	first, err := store.ReadResources(ctx, query)
	if err != nil || first.Total != 2 || len(first.Entries) != 1 || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	query.Cursor = first.NextCursor
	second, err := store.ReadResources(ctx, query)
	if err != nil || len(second.Entries) != 1 || second.Entries[0].LUID != "b" {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	query.ProjectName = "Ops/Reports"
	_, err = store.ReadResources(ctx, query)
	var invalid interface{ InvalidCacheCursor() bool }
	if !errors.As(err, &invalid) {
		t.Fatalf("changed-name cursor err=%v", err)
	}
	query.ProjectName = "Reports"
	_, err = store.ReplaceResourceScope(ctx, ResourceScopeReplacement{Environment: "test", Kind: "project", Source: "test", GeneratedAt: now.Add(time.Minute), Entries: []ResourceEntry{{LUID: "p", Name: "Renamed", ProjectPath: "Ops/Renamed"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReadResources(ctx, query)
	if !errors.As(err, &invalid) {
		t.Fatalf("changed-project snapshot cursor err=%v", err)
	}
}

func TestProjectNameFilterRejectsUnclassifiableIdentities(t *testing.T) {
	for _, payload := range []string{"", "not-json", `{}`, `{"project_luid":42}`, `{"project_luid":"unknown"}`} {
		t.Run(payload, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now().UTC()
			store := NewStore(t.TempDir(), func() time.Time { return now })
			for _, replacement := range []ResourceScopeReplacement{
				{Environment: "test", Kind: "project", Source: "test", GeneratedAt: now, Entries: []ResourceEntry{{LUID: "p", Name: "Reports"}}},
				{Environment: "test", Kind: "datasource", Source: "test", GeneratedAt: now, Entries: []ResourceEntry{{LUID: "d", Name: "Data", Payload: []byte(payload)}}},
			} {
				if _, err := store.ReplaceResourceScope(ctx, replacement); err != nil {
					t.Fatal(err)
				}
			}
			_, err := store.ReadResources(ctx, ResourceQuery{Environment: "test", Kind: "datasource", ProjectName: "Reports", Limit: 25})
			var unavailable interface{ CacheProjectRefreshRequired() bool }
			if !errors.As(err, &unavailable) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
