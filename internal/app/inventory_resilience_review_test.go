package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	projectinspect "github.com/ahillspace/tadx/actions/project/inspect"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func TestIncompleteFullInventoryFailsWithoutReplacingCache(t *testing.T) {
	for _, malformed := range []string{`<workbook id="broken" name="Broken" size="large"><project id="child"/></workbook>`, `<workbook id="broken" name="Broken"><project id="missing"/></workbook>`} {
		t.Run(malformed, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/projects"):
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="child" name="Ops"/></projects></tsResponse>`)
				case strings.HasSuffix(r.URL.Path, "/workbooks"):
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="4"/><workbooks><workbook id="a" name="Alpha"><project id="child"/></workbook>%s<workbook id="b" name="Beta"><project id="child"/></workbook><workbook id="c" name="Gamma"><project id="child"/></workbook></workbooks></tsResponse>`, malformed)
				default:
					http.Error(w, "unexpected request", 404)
				}
			}))
			defer server.Close()
			runtime := inventoryListRuntime(t, server)
			store := targetCacheFixture(t, runtime.configPath, runtime.now)
			seed, err := store.ReplaceResourceScope(context.Background(), cache.ResourceScopeReplacement{Environment: "production", Site: "team-site", Kind: "workbook", Source: "tableau-rest", GeneratedAt: runtime.now(), Entries: []cache.ResourceEntry{{LUID: "old", Name: "Old"}}})
			if err != nil {
				t.Fatal(err)
			}
			first, err := newRemoteContentCommands(runtime).ListWorkbooks(context.Background(), workbookops.ListInput{Environment: "production", All: true})
			if err == nil {
				t.Fatal("incomplete --all must fail")
			}
			if first.Page.Total != 3 || first.Page.NextCursor != "" || len(first.Workbooks) != 3 {
				t.Fatalf("first page = %#v", first)
			}
			count := requests.Load()
			if first.Source == nil || first.Source.Coverage != readsource.CoveragePartial || !strings.Contains(first.Source.CacheWarning, "skipped 1") {
				t.Fatalf("source = %#v", first.Source)
			}
			cached, err := store.ReadResources(context.Background(), cache.ResourceQuery{Environment: "production", Site: "team-site", Kind: "workbook", Limit: 10})
			if err != nil || cached.GenerationID != seed.GenerationID || len(cached.Entries) != 1 || cached.Entries[0].LUID != "old" {
				t.Fatalf("cache changed: %#v %v", cached, err)
			}
			if requests.Load() != count {
				t.Fatal("cache read contacted Tableau")
			}
		})
	}
}

func TestProjectInspectCacheRetainsCanonicalPathAfterLiveList(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		} else {
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="root" name="Department"/><project id="child" name="Ops" parentProjectId="root" description="Keep this detail"/></projects></tsResponse>`)
		}
	}))
	defer server.Close()
	commands := newRemoteContentCommands(inventoryListRuntime(t, server))
	if _, err := commands.ListProjects(context.Background(), projectlist.Input{Environment: "production", All: true}); err != nil {
		t.Fatal(err)
	}
	got, err := commands.InspectProject(context.Background(), projectinspect.Input{Environment: "production", Cache: true, Selector: identity.Selector{LUID: "child"}})
	if err != nil || got.Project.Path != "Department/Ops" || got.Project.Description != "Keep this detail" {
		t.Fatalf("cache inspect = %#v %v", got, err)
	}
}

func TestProjectInspectCacheRetainsCanonicalPathAfterFullRefresh(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	runtime := inventoryListRuntime(t, server)
	store := targetCacheFixture(t, runtime.configPath, runtime.now)
	writer, err := store.BeginGeneration(ctx, cache.GenerationMetadata{Environment: "production", Site: "team-site", GeneratedAt: runtime.now(), Source: "tableau-rest", RequestedScopes: []string{"projects"}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.WriteBatch(ctx, cache.Batch{Scope: "projects", Columns: []string{"id", "name", "parent_project_id", "description", "owner_id", "list_payload"}, Rows: [][]any{
		{"root", "Department", "", "", "", `{}`},
		{"child", "Ops", "root", "Keep this detail", "owner", `{"luid":"child","name":"Ops","parent_luid":"root","description":"Keep this detail"}`},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.CompleteScopes(ctx, []string{"projects"}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := newRemoteContentCommands(runtime).InspectProject(ctx, projectinspect.Input{Environment: "production", Cache: true, Selector: identity.Selector{LUID: "child"}})
	if err != nil || got.Project.Path != "Department/Ops" || got.Project.Description != "Keep this detail" || got.Project.OwnerLUID != "owner" {
		t.Fatalf("cache inspect = %#v %v", got, err)
	}
}

func TestInventoryKeepsHealthyBranchesWhenProjectHierarchyIsMalformed(t *testing.T) {
	entries, skipped, err := inventoryResourceEntries(tableaucache.InventorySnapshot{
		Scope: tableaucache.ScopeWorkbooks,
		Dependencies: []tableaucache.InventoryTable{{Scope: tableaucache.ScopeProjects, Rows: [][]any{
			{"healthy", "Ops", ""}, {"slash", "Ops/Reports", ""}, {"orphan", "Orphan", "missing"},
		}}},
		Rows: [][]any{
			{"good", "Good", "healthy", "", nil, "", `{}`},
			{"good-slash", "Slash Project", "slash", "", nil, "", `{}`},
			{"bad-orphan", "Bad Orphan", "orphan", "", nil, "", `{}`},
		},
	}, "dev", "site", time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC))
	if err != nil || skipped != 1 || len(entries) != 2 || entries[0].LUID != "good" || entries[0].ProjectPath != "Ops" || entries[1].LUID != "good-slash" || entries[1].ProjectPath != "Ops/Reports" {
		t.Fatalf("entries = %#v skipped = %d err = %v", entries, skipped, err)
	}
}
