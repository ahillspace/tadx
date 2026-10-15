package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestCacheStatusUninitializedThroughCLI(t *testing.T) {
	options := app.Options{ConfigPath: writePhaseOneConfig(t, "https://unused.example")}
	for _, full := range []bool{false, true} {
		args := []string{"cache", "status", "--environment", "production"}
		if full {
			args = append(args, "--full")
		}
		out := runGroupOneCLI(t, options, args...)
		decoded, err := toon.Decode([]byte(out))
		if err != nil {
			t.Fatal(err)
		}
		help := decoded.(map[string]any)["help"].([]any)
		want := commandhint.BindConfig("tadx cache refresh --environment production", options.ConfigPath)
		if !strings.Contains(out, "status: uninitialized") || len(help) != 1 || help[0] != want || strings.Contains(out, "sql:") || strings.Contains(out, "0001-") {
			t.Fatalf("uninitialized status:\n%s", out)
		}
	}
}

func TestCachePermissionDenialRetainsInventoryThroughCLI(t *testing.T) {
	for _, denyAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("all_denied_%t", denyAll), func(t *testing.T) {
			var requests atomic.Int32
			server := cacheResilienceServer(t, &requests, http.StatusForbidden, denyAll)
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			for _, full := range []bool{false, true} {
				args := []string{"cache", "refresh", "--environment", "production", "--scope", "workbooks,permissions"}
				if full {
					args = append(args, "--full")
				}
				out := runGroupOneCLI(t, options, args...)
				if !strings.Contains(out, "status: partial") || !strings.Contains(out, "permission") || !strings.Contains(out, "403") || strings.Contains(out, "complete: true") {
					t.Fatalf("partial refresh:\n%s", out)
				}
			}
			before := requests.Load()
			status := runGroupOneCLI(t, options, "cache", "status", "--environment", "production", "--full")
			if !strings.Contains(status, "status: partial") || !strings.Contains(status, "complete: false") || !strings.Contains(status, "permission") {
				t.Fatalf("persisted partial status:\n%s", status)
			}
			inventory := runGroupOneCLI(t, options, "content", "workbook", "list", "--environment", "production", "--cache", "--full")
			if !strings.Contains(inventory, "Allowed") || !strings.Contains(inventory, "Blocked") || !strings.Contains(inventory, "coverage: complete") || requests.Load() != before {
				t.Fatalf("cached inventory requests=%d/%d:\n%s", before, requests.Load(), inventory)
			}
			db, err := sql.Open("sqlite", filepath.Join(filepath.Dir(options.ConfigPath), targetCacheFixture(t, options.ConfigPath, nil).RelativePath()))
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
			exit := app.Run(context.Background(), []string{"admin", "permission", "inspect", "--environment", "production", "--kind", "workbook", "--id", "blocked", "--cache"}, &out, options)
			if exit == 0 || requests.Load() != before {
				t.Fatalf("unsupported cached permission read must not report empty rules: exit=%d\n%s", exit, out.String())
			}
		})
	}
}

func TestCachePermissionAuthenticationFailureStillFailsThroughCLI(t *testing.T) {
	var requests atomic.Int32
	server := cacheResilienceServer(t, &requests, http.StatusUnauthorized, false)
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	runGroupOneCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "workbooks")
	store := targetCacheFixture(t, options.ConfigPath, nil)
	selection := cache.Selection{Environment: "production", SiteSelected: true}
	before, err := store.Status(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	exit := app.Run(context.Background(), []string{"cache", "refresh", "--environment", "production", "--scope", "workbooks,permissions"}, &out, options)
	if exit == 0 || !strings.Contains(out.String(), "401") {
		t.Fatalf("authentication failure exit=%d:\n%s", exit, out.String())
	}
	status := runGroupOneCLI(t, options, "cache", "status", "--environment", "production")
	if !strings.Contains(status, "status: current") || !strings.Contains(status, before.GenerationID) {
		t.Fatalf("failed refresh replaced the previous generation:\n%s", status)
	}
}

func TestCachePersonalSpacePermissionsThroughCLI(t *testing.T) {
	for _, denyProject := range []bool{false, true} {
		t.Run(fmt.Sprintf("project_denied_%t", denyProject), func(t *testing.T) {
			var requests, personalPermissions, projectPermissions atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/projects"):
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
				case strings.HasSuffix(r.URL.Path, "/workbooks"):
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="3"/><workbooks><workbook id="personal" name="Private analysis"><location id="space-1" type="PersonalSpace"/><owner id="user-1"/></workbook><workbook id="allowed" name="Shared analysis"><location id="project-1" type="Project"/><project id="project-1"/></workbook><workbook id="project" name="Restricted analysis"><location id="project-1" type="Project"/><project id="project-1"/></workbook></workbooks></tsResponse>`)
				case strings.HasSuffix(r.URL.Path, "/personal/permissions"):
					personalPermissions.Add(1)
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, `<tsResponse><error code="403004"><summary>Forbidden</summary><detail>Personal Space permissions are unavailable.</detail></error></tsResponse>`)
				case strings.HasSuffix(r.URL.Path, "/project/permissions") && denyProject:
					projectPermissions.Add(1)
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, `<tsResponse><error code="403004"><summary>Forbidden</summary><detail>Workbook permissions are unavailable.</detail></error></tsResponse>`)
				case strings.HasSuffix(r.URL.Path, "/allowed/permissions"), strings.HasSuffix(r.URL.Path, "/project/permissions"):
					projectPermissions.Add(1)
					id := "allowed"
					if strings.HasSuffix(r.URL.Path, "/project/permissions") {
						id = "project"
					}
					_, _ = fmt.Fprintf(w, `<tsResponse><permissions><workbook id="%s"/><granteeCapabilities><user id="user-1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`, id)
				default:
					for _, scope := range []string{"users", "groups", "datasources", "flows", "views"} {
						if strings.HasSuffix(r.URL.Path, "/"+scope) {
							_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><%s/></tsResponse>`, scope)
							return
						}
					}
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			for _, explicit := range []bool{false, true} {
				for _, full := range []bool{false, true} {
					args := []string{"cache", "refresh", "--environment", "production"}
					if explicit {
						args = append(args, "--scope", "permissions")
					}
					if full {
						args = append(args, "--full", "--json")
					}
					beforeProjects := projectPermissions.Load()
					out := runGroupOneCLI(t, options, args...)
					if personalPermissions.Load() != 0 || projectPermissions.Load()-beforeProjects != 2 {
						t.Fatalf("permission requests: personal=%d project=%d; output:\n%s", personalPermissions.Load(), projectPermissions.Load()-beforeProjects, out)
					}
					var result map[string]any
					if full {
						if err := json.Unmarshal([]byte(out), &result); err != nil {
							t.Fatal(err)
						}
					} else {
						decoded, err := toon.Decode([]byte(out))
						if err != nil {
							t.Fatal(err)
						}
						result = decoded.(map[string]any)
					}
					if (result["status"] == "partial") != denyProject || strings.Contains(fmt.Sprint(result["warnings"]), "HTTP 403") != denyProject {
						t.Fatalf("refresh denyProject=%t explicit=%t full=%t:\n%s", denyProject, explicit, full, out)
					}
				}
			}
			before := requests.Load()
			store := targetCacheFixture(t, options.ConfigPath, nil)
			status, err := store.Status(t.Context(), cache.Selection{Environment: "production", SiteSelected: true})
			if err != nil || status.Complete == denyProject {
				t.Fatalf("persisted completeness=%t err=%v", status.Complete, err)
			}
			statusOutput := runGroupOneCLI(t, options, "cache", "status", "--environment", "production", "--full")
			if strings.Contains(statusOutput, "status: partial") != denyProject {
				t.Fatalf("persisted status:\n%s", statusOutput)
			}
			inventory := runGroupOneCLI(t, options, "content", "workbook", "list", "--environment", "production", "--cache", "--full")
			search := runGroupOneCLI(t, options, "search", "Private", "--type", "workbook", "--environment", "production", "--cache", "--full")
			if !strings.Contains(inventory, "Private analysis") || !strings.Contains(inventory, "Shared analysis") || !strings.Contains(inventory, "Restricted analysis") || !strings.Contains(inventory, "coverage: complete") || !strings.Contains(search, "personal") || requests.Load() != before {
				t.Fatalf("local inventory/search requests=%d/%d:\n%s\n%s", before, requests.Load(), inventory, search)
			}
			db, err := sql.Open("sqlite", filepath.Join(filepath.Dir(options.ConfigPath), store.RelativePath()))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var personalRules, projectRules int
			if err := db.QueryRow(`SELECT count(*) FROM permissions WHERE content_id='personal'`).Scan(&personalRules); err != nil || personalRules != 0 {
				t.Fatalf("personal permission rows=%d err=%v", personalRules, err)
			}
			wantRules := 2
			if denyProject {
				wantRules = 1
			}
			if err := db.QueryRow(`SELECT count(*) FROM permissions`).Scan(&projectRules); err != nil || projectRules != wantRules {
				t.Fatalf("permission rows=%d want=%d err=%v", projectRules, wantRules, err)
			}
		})
	}
}

func cacheResilienceOptions(t *testing.T, server *httptest.Server) app.Options {
	t.Helper()
	t.Setenv("PROD_PAT_NAME", "test-pat-name")
	t.Setenv("PROD_PAT_SECRET", "test-pat-secret")
	return app.Options{ConfigPath: writePhaseOneConfig(t, server.URL), HTTPClient: server.Client()}
}

func cacheResilienceServer(t *testing.T, requests *atomic.Int32, deniedStatus int, denyAll bool) *httptest.Server {
	t.Helper()
	return tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
