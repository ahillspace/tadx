package app

import (
	"context"
	"fmt"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/readsource"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
			first, err := newRemoteContentCommands(runtime).dependencies().WorkbookLister.ListWorkbooks(context.Background(), workbookops.ListInput{Environment: "production", All: true})
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
