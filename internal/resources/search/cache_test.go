package search

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/errs"
)

func executeSearchAction(ctx context.Context, source searchaction.Source, input searchaction.Input) (searchaction.Output, error) {
	types, err := searchaction.ValidateInput(input)
	if err != nil {
		return searchaction.Output{}, err
	}
	return searchaction.Execute(ctx, source, input, types)
}

func TestCacheGlobalSearchIncludesReadThroughResourcesWithoutGeneration(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{{Environment: "dev", Site: "site", Kind: "workbook", LUID: "wb-1", Name: "Sales", Coverage: "summary", ObservedAt: now}}); err != nil {
		t.Fatal(err)
	}
	out, err := executeSearchAction(t.Context(), CacheSource{Store: store}, searchaction.Input{Terms: "sales", Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].LUID != "wb-1" || out.Generation != nil {
		t.Fatalf("output=%+v", out)
	}
}

func TestCacheGlobalSearchRejectsExplicitUnavailableType(t *testing.T) {
	store := cache.NewStore(t.TempDir(), time.Now)
	_, err := executeSearchAction(t.Context(), CacheSource{Store: store}, searchaction.Input{Type: "metric", Environment: "dev", Site: "site", SiteResolved: true, Cache: true})
	if structured, ok := errors.AsType[*errs.Error](err); !ok || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%v", err)
	}
}

func TestCacheSearchReportsScopeGeneration(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	ctx := t.Context()
	if _, err := store.Replace(ctx, cache.Generation{ID: "full-old", Environment: "dev", Site: "site", GeneratedAt: now.Add(-48 * time.Hour), Complete: true, Source: "tableau-rest", Scopes: []string{"workbooks"}, Records: []cache.Record{{LUID: "old", Kind: "workbook", Name: "Old"}}}); err != nil {
		t.Fatal(err)
	}
	replacement, err := store.ReplaceResourceScope(ctx, cache.ResourceScopeReplacement{Environment: "dev", Site: "site", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now, Entries: []cache.ResourceEntry{{LUID: "new", Name: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := executeSearchAction(ctx, CacheSource{Store: store}, searchaction.Input{Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Generation == nil || out.Generation.ID != replacement.GenerationID || out.Generation.Stale {
		t.Fatalf("scope provenance = %#v", out.Generation)
	}
	if err := store.UpsertResources(ctx, []cache.ResourceEntry{{Environment: "dev", Site: "site", Kind: "workbook", LUID: "new", Name: "Updated", Coverage: "summary", ObservedAt: now.Add(time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	out, err = executeSearchAction(ctx, CacheSource{Store: store}, searchaction.Input{Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Generation != nil || len(out.Warnings) == 0 {
		t.Fatalf("partial provenance = %#v", out)
	}
}

func TestReadThroughCacheSearchContinuesAcrossObservationTimes(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	entries := make([]cache.ResourceEntry, 101)
	for i := range entries {
		entries[i] = cache.ResourceEntry{Environment: "dev", Site: "site", Kind: "workbook", LUID: fmt.Sprintf("wb-%03d", i), Name: fmt.Sprintf("Workbook %03d", i), Coverage: "summary", ObservedAt: now.Add(time.Duration(i) * time.Minute)}
	}
	if err := store.UpsertResources(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	lister := &cacheSearchLister{store: store, environment: "dev", site: "site"}
	first, err := lister.List(t.Context(), "workbook", "", 100)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page: %#v %v", first, err)
	}
	second, err := lister.List(t.Context(), "workbook", first.NextCursor, 100)
	if err != nil || len(second.Items) != 1 {
		t.Fatalf("second page: %#v %v", second, err)
	}
}
