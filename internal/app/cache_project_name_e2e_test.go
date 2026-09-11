package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
)

func TestCacheDatasourceProjectNameThroughCLI(t *testing.T) {
	for _, coverage := range []string{"complete", "missing-projects", "partial-projects", "partial-datasources", "missing-identity"} {
		t.Run(coverage, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unexpected network", 500) }))
			defer server.Close()
			options := diagnosticOptions(t, server)
			now := time.Now().UTC()
			store := targetCacheFixture(t, options.ConfigPath, func() time.Time { return now })
			projects := []cache.ResourceEntry{{LUID: "literal", Name: "Ops/Reports", ProjectPath: "Ops/Reports"}, {LUID: "parent", Name: "Ops", ProjectPath: "Ops"}, {LUID: "nested", Name: "Reports", ProjectPath: "Ops/Reports"}, {LUID: "another", Name: "Reports", ProjectPath: "Other/Reports"}}
			if coverage != "missing-projects" {
				if _, err := store.ReplaceResourceScope(context.Background(), cache.ResourceScopeReplacement{Environment: "test", Kind: "project", Source: "test", GeneratedAt: now, Entries: projects}); err != nil {
					t.Fatal(err)
				}
				if coverage == "partial-projects" {
					entry := projects[0]
					entry.Environment = "test"
					entry.Kind = "project"
					entry.ObservedAt = now
					entry.Coverage = "summary"
					if err := store.UpsertResources(context.Background(), []cache.ResourceEntry{entry}); err != nil {
						t.Fatal(err)
					}
				}
			}
			var entries []cache.ResourceEntry
			for _, project := range projects {
				item := datasourcelist.Datasource{LUID: "ds-" + project.LUID, Name: "Data-" + project.LUID, ProjectLUID: project.LUID, ProjectName: "untrusted-stale-name", ProjectPath: project.ProjectPath}
				if coverage == "missing-identity" {
					item.ProjectLUID = ""
				}
				payload, err := json.Marshal(item)
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, cache.ResourceEntry{LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath, ProjectLUID: item.ProjectLUID, Payload: payload})
			}
			if _, err := store.ReplaceResourceScope(context.Background(), cache.ResourceScopeReplacement{Environment: "test", Kind: "datasource", Source: "test", GeneratedAt: now, Entries: entries}); err != nil {
				t.Fatal(err)
			}
			if coverage == "partial-datasources" {
				entry := entries[0]
				entry.Environment, entry.Kind, entry.Coverage, entry.ObservedAt = "test", "datasource", "summary", now
				if err := store.UpsertResources(context.Background(), []cache.ResourceEntry{entry}); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"Reports", "Ops/Reports", "Absent"} {
				var out bytes.Buffer
				code := app.Run(context.Background(), []string{"content", "datasource", "list", "--project-name", name, "--cache", "--environment", "test", "--full"}, &out, options)
				if coverage != "complete" {
					if code == 0 || !strings.Contains(out.String(), "cache refresh") || strings.Contains(out.String(), "without --cache") {
						t.Fatalf("code=%d output=%s", code, out.String())
					}
					continue
				}
				if code != 0 {
					t.Fatalf("code=%d output=%s", code, out.String())
				}
				for _, project := range projects {
					if strings.Contains(out.String(), "ds-"+project.LUID) != (project.Name == name) {
						t.Fatalf("project filter %q output=%s", name, out.String())
					}
				}
			}
			if calls != 0 {
				t.Fatalf("cache made %d network calls", calls)
			}
		})
	}
}
