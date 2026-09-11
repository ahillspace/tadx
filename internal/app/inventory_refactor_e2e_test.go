package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
)

func TestLimitedLiveInventoryDoesNotCollectOrDependOnCacheThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project", "user", "group"} {
		t.Run(kind, func(t *testing.T) {
			reads := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/auth/signin") {
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
					return
				}
				resource, total := kind, 205
				if strings.HasSuffix(r.URL.Path, "/projects") && kind != "project" {
					resource, total = "project", 1
				} else {
					reads++
					if r.URL.Query().Get("pageSize") != "3" || r.URL.Query().Get("pageNumber") != "1" {
						t.Errorf("limited read expanded provider request: %s", r.URL.RawQuery)
					}
				}
				size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
				if size <= 0 {
					size = 1000
				}
				_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%d" totalAvailable="%d"/><%ss>`, size, total, resource)
				for i := 0; i < min(size, total); i++ {
					_, _ = fmt.Fprintf(w, `<%s id="%s-%03d" name="Item %03d" siteRole="Viewer" type="hyper" fileType="tflx"><project id="project-000" name="Item 000"/><owner id="user-1"/></%s>`, resource, resource, i, i, resource)
				}
				_, _ = fmt.Fprintf(w, `</%ss></tsResponse>`, resource)
			}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			// An unusable SQLite location must not participate in a limited live answer.
			if err := os.WriteFile(filepath.Join(filepath.Dir(options.ConfigPath), "catalog"), []byte("not a directory"), 0600); err != nil {
				t.Fatal(err)
			}
			root := "content"
			if kind == "user" || kind == "group" {
				root = "admin"
			}
			var out strings.Builder
			exit := app.Run(context.Background(), []string{root, kind, "list", "--environment", "production", "--limit", "3"}, &out, options)
			if exit != 0 || reads != 1 || !strings.Contains(out.String(), "returned: 3") || !strings.Contains(out.String(), "more_available: true") {
				t.Fatalf("reads=%d exit=%d output=%s", reads, exit, out.String())
			}
		})
	}
}

func TestAdminAllRejectsPartialCacheThroughCLI(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("cache contacted remote %s", r.URL)
				w.WriteHeader(500)
			}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			store := targetCacheFixture(t, options.ConfigPath, nil)
			err := store.UpsertResources(context.Background(), []cache.ResourceEntry{{Environment: "production", Kind: kind, LUID: "known", Name: "Known", Coverage: "summary", ObservedAt: time.Now(), Payload: []byte(`{"luid":"known","name":"Known"}`)}})
			if err != nil {
				t.Fatal(err)
			}
			for _, all := range []bool{false, true} {
				var out strings.Builder
				args := []string{"admin", kind, "list", "--cache", "--environment", "production"}
				if all {
					args = append(args, "--all")
				}
				exit := app.Run(context.Background(), args, &out, options)
				if all && exit == 0 || !all && exit != 0 {
					t.Fatalf("all=%t exit=%d output=%s", all, exit, out.String())
				}
			}
		})
	}
}

func TestCachedProjectPathIsOpaqueThroughCLI(t *testing.T) {
	server, _ := slashInventoryServer(t)
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	runGroupOneCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "projects")
	for _, selector := range [][]string{{"--project", "Ops/Reports"}, {"--project-id", "slash"}, {"--project-id", "nested"}, {"--project", "does/not/exist"}} {
		var out strings.Builder
		args := append([]string{"content", "project", "inspect", "--environment", "production", "--cache"}, selector...)
		exit := app.Run(context.Background(), args, &out, options)
		if selector[0] == "--project-id" {
			if exit != 0 {
				t.Fatalf("%v: %s", selector, out.String())
			}
		} else if exit == 0 {
			t.Fatalf("ambiguous or missing path succeeded: %s", out.String())
		}
	}
	store := targetCacheFixture(t, options.ConfigPath, nil)
	_, err := store.ReplaceResourceScope(context.Background(), cache.ResourceScopeReplacement{Environment: "production", Kind: "project", Source: "test", GeneratedAt: time.Now(), Entries: []cache.ResourceEntry{{LUID: "literal-only", Name: "Ops/Reports", ProjectPath: "Ops/Reports"}}})
	if err != nil {
		t.Fatal(err)
	}
	output := runGroupOneCLI(t, options, "content", "project", "inspect", "--environment", "production", "--cache", "--project", "Ops/Reports")
	if !strings.Contains(output, "literal-only") {
		t.Fatalf("literal-only opaque path did not resolve: %s", output)
	}
}

func TestOldCacheSchemaRequiresExplicitRefreshThroughCLI(t *testing.T) {
	server, _ := slashInventoryServer(t)
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	runGroupOneCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "projects")
	cachePath := filepath.Join(filepath.Dir(options.ConfigPath), targetCacheFixture(t, options.ConfigPath, nil).RelativePath())
	db, err := sql.Open("sqlite", cachePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE catalog_schema SET version=5,signature='tadx-catalog-v5'; PRAGMA user_version=5`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	var out strings.Builder
	exit := app.Run(context.Background(), []string{"content", "project", "list", "--cache", "--environment", "production"}, &out, options)
	if exit == 0 || !strings.Contains(out.String(), "refresh") {
		t.Fatalf("old schema silently read or unclear recovery: exit=%d %s", exit, out.String())
	}
	before, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	runGroupOneCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "projects")
	after, err := os.ReadFile(options.ConfigPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("refresh changed configuration")
	}
	runGroupOneCLI(t, options, "content", "project", "list", "--cache", "--environment", "production")
}
