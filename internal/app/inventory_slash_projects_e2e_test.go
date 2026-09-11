package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestCachedContentSelectorsRejectAmbiguousProjectPathsThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			server, requests := slashInventoryServer(t)
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			// Projects are implicit in this refresh; their identities must still
			// constrain content selectors without requiring a separate project list.
			runGroupOneCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", kind+"s")
			before := requests.Load()
			var out strings.Builder
			exit := app.Run(context.Background(), []string{"content", kind, "inspect", "--environment", "production", "--cache", "--name", "Revenue", "--project", "Ops/Reports"}, &out, options)
			if exit == 0 || !strings.Contains(out.String(), "ambiguous") || !strings.Contains(out.String(), "slash") || !strings.Contains(out.String(), "nested") {
				t.Fatalf("ambiguous cached project path: exit=%d\n%s", exit, out.String())
			}
			exact := runGroupOneCLI(t, options, "content", kind, "inspect", "--environment", "production", "--cache", "--id", kind+"-direct")
			if !strings.Contains(exact, kind+"-direct") || requests.Load() != before {
				t.Fatalf("exact cached LUID failed or contacted Tableau:\n%s", exact)
			}
		})
	}
}

func TestSlashProjectInventoryRetainsContentThroughCLI(t *testing.T) {
	for _, kind := range []string{"project", "workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			server, requests := slashInventoryServer(t)
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			out := runGroupOneCLI(t, options, "content", kind, "list", "--environment", "production", "--all", "--full")
			assertSlashInventory(t, kind, out)
			before := requests.Load()
			cached := runGroupOneCLI(t, options, "content", kind, "list", "--environment", "production", "--cache", "--full")
			assertSlashInventory(t, kind, cached)
			if requests.Load() != before {
				t.Fatal("cache read contacted Tableau")
			}
		})
	}
}

func TestCacheRefreshRetainsSlashProjectContentThroughCLI(t *testing.T) {
	server, requests := slashInventoryServer(t)
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	out := runGroupOneCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "projects,workbooks,datasources,flows", "--full")
	if !strings.Contains(out, "status: refreshed") || !strings.Contains(out, "complete: true") {
		t.Fatalf("refresh:\n%s", out)
	}
	before := requests.Load()
	for _, kind := range []string{"project", "workbook", "datasource", "flow"} {
		out = runGroupOneCLI(t, options, "content", kind, "list", "--environment", "production", "--cache", "--full")
		assertSlashInventory(t, kind, out)
	}
	if requests.Load() != before {
		t.Fatal("cache reads contacted Tableau")
	}
}

func assertSlashInventory(t *testing.T, kind, out string) {
	t.Helper()
	wantIDs := []string{kind + "-direct", kind + "-descendant", kind + "-nested"}
	if kind == "project" {
		wantIDs = []string{"slash", "slash-child", "root", "nested", "nested-child"}
	}
	for _, id := range wantIDs {
		if !strings.Contains(out, id) {
			t.Fatalf("%s inventory dropped %s:\n%s", kind, id, out)
		}
	}
	if strings.Contains(out, "skipped") || !strings.Contains(out, "coverage: complete") {
		t.Fatalf("valid slash hierarchy reported incomplete coverage:\n%s", out)
	}
}

func slashInventoryServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="5"/><projects><project id="slash" name="Ops/Reports"/><project id="slash-child" name="Team" parentProjectId="slash"/><project id="root" name="Ops"/><project id="nested" name="Reports" parentProjectId="root"/><project id="nested-child" name="Team" parentProjectId="nested"/></projects></tsResponse>`)
		default:
			for _, kind := range []string{"workbook", "datasource", "flow"} {
				if !strings.HasSuffix(r.URL.Path, "/"+kind+"s") {
					continue
				}
				_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="3"/><%ss>`, kind)
				for index, project := range []string{"slash", "slash-child", "nested-child"} {
					suffix := []string{"direct", "descendant", "nested"}[index]
					_, _ = fmt.Fprintf(w, `<%s id="%s-%s" name="Revenue" type="hyper" fileType="tflx"><project id="%s"/><owner id="user-1"/></%s>`, kind, kind, suffix, project, kind)
				}
				_, _ = fmt.Fprintf(w, `</%ss></tsResponse>`, kind)
				return
			}
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	return server, requests
}
