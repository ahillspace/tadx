package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/catalog"
)

func TestCatalogStatusUninitializedThroughCLI(t *testing.T) {
	options := app.Options{ConfigPath: writePhaseOneConfig(t, "https://unused.example")}
	for _, full := range []bool{false, true} {
		args := []string{"catalog", "status", "--environment", "production"}
		if full {
			args = append(args, "--full")
		}
		out := runGroupOneCLI(t, options, args...)
		if !strings.Contains(out, "status: uninitialized") || !strings.Contains(out, "tadx catalog refresh --environment production") || strings.Contains(out, "sql:") || strings.Contains(out, "0001-") {
			t.Fatalf("uninitialized status:\n%s", out)
		}
	}
}

func TestCatalogPermissionDenialRetainsInventoryThroughCLI(t *testing.T) {
	for _, denyAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("all_denied_%t", denyAll), func(t *testing.T) {
			var requests atomic.Int32
			server := catalogResilienceServer(t, &requests, http.StatusForbidden, denyAll)
			defer server.Close()
			options := catalogResilienceOptions(t, server)
			for _, full := range []bool{false, true} {
				args := []string{"catalog", "refresh", "--environment", "production", "--scope", "workbooks,permissions"}
				if full {
					args = append(args, "--full")
				}
				out := runGroupOneCLI(t, options, args...)
				if !strings.Contains(out, "status: partial") || !strings.Contains(out, "permission") || !strings.Contains(out, "403") || strings.Contains(out, "complete: true") {
					t.Fatalf("partial refresh:\n%s", out)
				}
			}
			before := requests.Load()
			status := runGroupOneCLI(t, options, "catalog", "status", "--environment", "production", "--full")
			if !strings.Contains(status, "status: partial") || !strings.Contains(status, "complete: false") || !strings.Contains(status, "permission") {
				t.Fatalf("persisted partial status:\n%s", status)
			}
			inventory := runGroupOneCLI(t, options, "content", "workbook", "list", "--environment", "production", "--catalog", "--full")
			if !strings.Contains(inventory, "Allowed") || !strings.Contains(inventory, "Blocked") || !strings.Contains(inventory, "coverage: complete") || requests.Load() != before {
				t.Fatalf("cached inventory requests=%d/%d:\n%s", before, requests.Load(), inventory)
			}
			db, err := sql.Open("sqlite", filepath.Join(filepath.Dir(options.ConfigPath), catalog.DatabasePath()))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var complete, rules int
			if err := db.QueryRow(`SELECT complete FROM generation_scopes WHERE scope='permissions'`).Scan(&complete); err != nil || complete != 0 {
				t.Fatalf("permission completeness=%d err=%v", complete, err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM permissions`).Scan(&rules); err != nil || (!denyAll && rules != 1) || (denyAll && rules != 0) {
				t.Fatalf("retained permission rules=%d err=%v", rules, err)
			}
			var out strings.Builder
			exit := app.Run(context.Background(), []string{"admin", "permission", "inspect", "--environment", "production", "--kind", "workbook", "--id", "blocked", "--catalog"}, &out, options)
			if exit == 0 || requests.Load() != before {
				t.Fatalf("unsupported cached permission read must not report empty rules: exit=%d\n%s", exit, out.String())
			}
		})
	}
}

func TestCatalogPermissionAuthenticationFailureStillFailsThroughCLI(t *testing.T) {
	var requests atomic.Int32
	server := catalogResilienceServer(t, &requests, http.StatusUnauthorized, false)
	defer server.Close()
	options := catalogResilienceOptions(t, server)
	runGroupOneCLI(t, options, "catalog", "refresh", "--environment", "production", "--scope", "workbooks")
	store := catalog.NewStore(filepath.Dir(options.ConfigPath), nil)
	selection := catalog.Selection{Environment: "production", SiteSelected: true}
	before, err := store.Status(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	exit := app.Run(context.Background(), []string{"catalog", "refresh", "--environment", "production", "--scope", "workbooks,permissions"}, &out, options)
	if exit == 0 || !strings.Contains(out.String(), "401") {
		t.Fatalf("authentication failure exit=%d:\n%s", exit, out.String())
	}
	status := runGroupOneCLI(t, options, "catalog", "status", "--environment", "production")
	if !strings.Contains(status, "status: current") || !strings.Contains(status, before.GenerationID) {
		t.Fatalf("failed refresh replaced the previous generation:\n%s", status)
	}
}

func catalogResilienceOptions(t *testing.T, server *httptest.Server) app.Options {
	t.Helper()
	t.Setenv("PROD_PAT_NAME", "test-pat-name")
	t.Setenv("PROD_PAT_SECRET", "test-pat-secret")
	return app.Options{ConfigPath: writePhaseOneConfig(t, server.URL), HTTPClient: server.Client()}
}

func catalogResilienceServer(t *testing.T, requests *atomic.Int32, deniedStatus int, denyAll bool) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><workbooks><workbook id="allowed" name="Allowed"><project id="project-1"/><owner id="user-1"/></workbook><workbook id="blocked" name="Blocked"><project id="project-1"/><owner id="user-1"/></workbook></workbooks></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/blocked/permissions") || (denyAll && strings.HasSuffix(r.URL.Path, "/permissions")):
			w.WriteHeader(deniedStatus)
			_, _ = fmt.Fprintf(w, `<tsResponse><error code="%d004"><summary>Permission request failed</summary><detail>Permission inspection is unavailable.</detail></error></tsResponse>`, deniedStatus)
		case strings.HasSuffix(r.URL.Path, "/allowed/permissions"):
			_, _ = io.WriteString(w, `<tsResponse><permissions><workbook id="allowed"/><granteeCapabilities><user id="user-1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
}
