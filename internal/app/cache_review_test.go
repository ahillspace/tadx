package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cache"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func TestCacheSearchReportsScopeGeneration(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	ctx := context.Background()
	if _, err := store.Replace(ctx, cache.Generation{ID: "full-old", Environment: "dev", Site: "site", GeneratedAt: now.Add(-48 * time.Hour), Complete: true, Source: "tableau-rest", Scopes: []string{"workbooks"}, Records: []cache.Record{{LUID: "old", Kind: "workbook", Name: "Old"}}}); err != nil {
		t.Fatal(err)
	}
	replacement, err := store.ReplaceResourceScope(ctx, cache.ResourceScopeReplacement{Environment: "dev", Site: "site", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now, Entries: []cache.ResourceEntry{{LUID: "new", Name: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := searchaction.New(cacheGlobalSearchSource{store: store}).Execute(ctx, searchaction.Input{Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Generation == nil || out.Generation.ID != replacement.GenerationID || out.Generation.Stale {
		t.Fatalf("scope provenance = %#v", out.Generation)
	}
	if err := store.UpsertResources(ctx, []cache.ResourceEntry{{Environment: "dev", Site: "site", Kind: "workbook", LUID: "new", Name: "Updated", Coverage: "summary", ObservedAt: now.Add(time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	out, err = searchaction.New(cacheGlobalSearchSource{store: store}).Execute(ctx, searchaction.Input{Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Generation != nil || len(out.Warnings) == 0 {
		t.Fatalf("partial provenance = %#v", out)
	}
}

func TestSingleSourceListSearchPropagatesTotal(t *testing.T) {
	adapter := &completeLiveSearchAdapter{lister: &completeListPagerFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "One"}}, Total: 42, NextCursor: "more"}}}}
	out, err := searchaction.New(globalSearchSource{lists: adapter}).Execute(context.Background(), searchaction.Input{Environment: "dev", SiteResolved: true, Type: "workbook", Limit: 1})
	if err != nil || out.Page.Total != 42 {
		t.Fatalf("list search = %#v, %v", out, err)
	}
}

func TestGroupedSearchRetainsUnresolvedTruncationAcrossActionPages(t *testing.T) {
	for _, unresolved := range []bool{false, true} {
		pages := []resourcesearch.Page{
			{Items: []resourcesearch.Item{{LUID: "g", Type: "group", Name: "Group"}}, MoreAvailable: unresolved},
			{Items: []resourcesearch.Item{{LUID: "u1", Type: "user", Name: "User1"}}, NextCursor: "users-next", MoreAvailable: true},
			{Items: []resourcesearch.Item{{LUID: "u2", Type: "user", Name: "User2"}}},
		}
		adapter := &completeLiveSearchAdapter{lister: &completeListPagerFake{pages: pages}}
		out, err := searchaction.New(globalSearchSource{lists: adapter}).Execute(context.Background(), searchaction.Input{Environment: "dev", SiteResolved: true, Type: "admin", Limit: 200})
		if err != nil || out.Page.MoreAvailable != unresolved || len(out.Items) != 3 {
			t.Fatalf("unresolved=%t out=%+v err=%v", unresolved, out, err)
		}
	}
}

func TestReadThroughCacheSearchContinuesAcrossObservationTimes(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	entries := make([]cache.ResourceEntry, 101)
	for i := range entries {
		entries[i] = cache.ResourceEntry{Environment: "dev", Site: "site", Kind: "workbook", LUID: fmt.Sprintf("wb-%03d", i), Name: fmt.Sprintf("Workbook %03d", i), Coverage: "summary", ObservedAt: now.Add(time.Duration(i) * time.Minute)}
	}
	if err := store.UpsertResources(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	lister := &cacheSearchLister{store: store, environment: "dev", site: "site"}
	first, err := lister.List(context.Background(), "workbook", "", 100)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page: %#v %v", first, err)
	}
	second, err := lister.List(context.Background(), "workbook", first.NextCursor, 100)
	if err != nil || len(second.Items) != 1 {
		t.Fatalf("second page: %#v %v", second, err)
	}
}

func TestInventoryPreservesSlashProjectDisplayName(t *testing.T) {
	projects, err := inventoryProjects(tableaucache.InventorySnapshot{Scope: tableaucache.ScopeProjects, Rows: [][]any{{"p", "A/B", ""}}})
	if err != nil || projects["p"].path != "A/B" {
		t.Fatalf("slash project = %#v, error = %v", projects, err)
	}
}
