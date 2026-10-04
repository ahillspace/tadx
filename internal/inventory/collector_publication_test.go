package inventory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corecache "github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

type executorFunc func(context.Context, tableaucache.Request) (tableaucache.Response, error)

func (f executorFunc) Do(ctx context.Context, request tableaucache.Request) (tableaucache.Response, error) {
	return f(ctx, request)
}

func TestFilteredInventoryUpsertsWithoutReplacingScope(t *testing.T) {
	observed := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := corecache.NewStore(t.TempDir(), func() time.Time { return observed })
	ctx := t.Context()
	if err := store.UpsertResources(ctx, []corecache.ResourceEntry{{Environment: "dev", Site: "site", Kind: "project", LUID: "old", Name: "Old", Coverage: "summary", ObservedAt: observed, Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	var filters []string
	executor := executorFunc(func(_ context.Context, request tableaucache.Request) (tableaucache.Response, error) {
		filters = append(filters, request.Query.Get("filter"))
		name := "New"
		if request.Query.Get("filter") == "" {
			name = "Old"
		}
		body := fmt.Sprintf(`<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="new" name="%s"/></projects></tsResponse>`, name)
		return tableaucache.Response{StatusCode: 200, Body: []byte(body), TableauRequestID: "request-projects"}, nil
	})
	result, err := Collect(ctx, executor, store, tableaucache.ScopeProjects, "dev", "site", observed, Options{Filter: "name:eq:New"})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.FinalRequestID(); got != "request-projects" {
		t.Fatalf("request ID = %q", got)
	}
	if len(filters) != 2 || filters[0] != "" || filters[1] != "name:eq:New" {
		t.Fatalf("filters = %#v", filters)
	}
	if len(result.Entries) != 1 || result.Entries[0].LUID != "new" {
		t.Fatalf("entries = %#v", result.Entries)
	}
	source, help := result.SourceAndHelp(observed, func() time.Time { return observed.Add(time.Minute) })
	if source.Mode != readsource.Tableau || source.Coverage != readsource.CoverageComplete || source.CacheRefreshed || help != "" {
		t.Fatalf("filtered source = %#v, help = %q", source, help)
	}
	cached, err := store.ReadResources(ctx, corecache.ResourceQuery{Environment: "dev", Site: "site", Kind: "project", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if cached.Total != 2 || cached.Coverage == readsource.CoverageComplete {
		t.Fatalf("filtered cache = %#v", cached)
	}
}

func TestUnfilteredInventoryReplacesScopeAndPublishesGeneration(t *testing.T) {
	observed := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := corecache.NewStore(t.TempDir(), func() time.Time { return observed })
	ctx := t.Context()
	if err := store.UpsertResources(ctx, []corecache.ResourceEntry{{Environment: "dev", Site: "site", Kind: "project", LUID: "old", Name: "Old", Coverage: "summary", ObservedAt: observed, Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	executor := executorFunc(func(_ context.Context, request tableaucache.Request) (tableaucache.Response, error) {
		if request.Query.Get("filter") != "" {
			t.Fatalf("unexpected filter: %v", request.Query)
		}
		body := `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="new" name="New"/></projects></tsResponse>`
		return tableaucache.Response{StatusCode: 200, Body: []byte(body), TableauRequestID: "request-projects"}, nil
	})
	result, err := Collect(ctx, executor, store, tableaucache.ScopeProjects, "dev", "site", observed)
	if err != nil {
		t.Fatal(err)
	}
	source, help := result.SourceAndHelp(observed, func() time.Time { return observed })
	if source.Mode != readsource.Tableau || source.Coverage != readsource.CoverageComplete || !source.CacheRefreshed || source.CacheGeneration == "" || help != "" {
		t.Fatalf("unfiltered source = %#v, help = %q", source, help)
	}
	cached, err := store.ReadResources(ctx, corecache.ResourceQuery{Environment: "dev", Site: "site", Kind: "project", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if cached.Total != 1 || cached.Entries[0].LUID != "new" || cached.Coverage != readsource.CoverageComplete || cached.GenerationID != source.CacheGeneration {
		t.Fatalf("replaced cache = %#v", cached)
	}
}

func TestSkippedInventoryRowsKeepPartialLiveResultWithoutPublishing(t *testing.T) {
	observed := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	store := corecache.NewStore(t.TempDir(), func() time.Time { return observed })
	executor := executorFunc(func(_ context.Context, _ tableaucache.Request) (tableaucache.Response, error) {
		body := `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="valid" name="Valid"/><project name="Invalid"/></projects></tsResponse>`
		return tableaucache.Response{StatusCode: 200, Body: []byte(body)}, nil
	})
	result, err := Collect(t.Context(), executor, store, tableaucache.ScopeProjects, "dev", "site", observed)
	if err != nil {
		t.Fatal(err)
	}
	source, help := result.SourceAndHelp(observed, func() time.Time { return observed })
	if source.Mode != readsource.Tableau || source.Coverage != readsource.CoveragePartial || source.CoverageReason != "malformed_records_skipped" || source.CacheRefreshed || !strings.Contains(help, "skipped 1 malformed project record") {
		t.Fatalf("partial source = %#v, help = %q", source, help)
	}
	if len(result.Entries) != 1 || result.Entries[0].LUID != "valid" {
		t.Fatalf("partial entries = %#v", result.Entries)
	}
}
