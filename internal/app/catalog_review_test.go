package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/catalog"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
)

func TestCatalogSearchReportsScopeGeneration(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(t.TempDir(), func() time.Time { return now })
	ctx := context.Background()
	if _, err := store.Replace(ctx, catalog.Generation{ID: "full-old", Environment: "dev", Site: "site", GeneratedAt: now.Add(-48 * time.Hour), Complete: true, Source: "tableau-rest", Scopes: []string{"workbooks"}, Records: []catalog.Record{{LUID: "old", Kind: "workbook", Name: "Old"}}}); err != nil {
		t.Fatal(err)
	}
	replacement, err := store.ReplaceResourceScope(ctx, catalog.ResourceScopeReplacement{Environment: "dev", Site: "site", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now, Entries: []catalog.ResourceEntry{{LUID: "new", Name: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := searchaction.New(catalogGlobalSearchSource{store: store}).Execute(ctx, searchaction.Input{Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Catalog: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Generation == nil || out.Generation.ID != replacement.GenerationID || out.Generation.Stale {
		t.Fatalf("scope provenance = %#v", out.Generation)
	}
	if err := store.UpsertResources(ctx, []catalog.ResourceEntry{{Environment: "dev", Site: "site", Kind: "workbook", LUID: "new", Name: "Updated", Coverage: "summary", ObservedAt: now.Add(time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	out, err = searchaction.New(catalogGlobalSearchSource{store: store}).Execute(ctx, searchaction.Input{Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Catalog: true})
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

func TestReadThroughCatalogSearchContinuesAcrossObservationTimes(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(t.TempDir(), func() time.Time { return now })
	entries := make([]catalog.ResourceEntry, 101)
	for i := range entries {
		entries[i] = catalog.ResourceEntry{Environment: "dev", Site: "site", Kind: "workbook", LUID: fmt.Sprintf("wb-%03d", i), Name: fmt.Sprintf("Workbook %03d", i), Coverage: "summary", ObservedAt: now.Add(time.Duration(i) * time.Minute)}
	}
	if err := store.UpsertResources(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	lister := &catalogSearchLister{store: store, environment: "dev", site: "site"}
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
	projects, err := inventoryProjects(tableaucatalog.InventorySnapshot{Scope: tableaucatalog.ScopeProjects, Rows: [][]any{{"p", "A/B", ""}}})
	if err != nil || projects["p"].path != "A/B" {
		t.Fatalf("slash project = %#v, error = %v", projects, err)
	}
}
