package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/catalog"
)

func TestCompactInventoryColumnsRemainStableAcrossLimitsThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project", "user", "group"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Errorf("catalog read contacted %s", r.URL) }))
			defer server.Close()
			options := catalogResilienceOptions(t, server)
			store := catalog.NewStore(filepath.Dir(options.ConfigPath), nil)
			entries := []catalog.ResourceEntry{
				{Environment: "production", Kind: kind, LUID: "a", Name: "Alpha", Coverage: "summary", ObservedAt: time.Now(), Payload: []byte(`{"luid":"a","name":"Alpha","project_luid":"project-1","project_name":"Ops","project_path":"Ops","parent_luid":"parent-1","type":"hyper","file_type":"tflx","content_url":"alpha","updated_at":"2026-09-01T00:00:00Z","site_role":"Viewer","domain":"local"}`)},
				{Environment: "production", Kind: kind, LUID: "b", Name: "Beta", Coverage: "summary", ObservedAt: time.Now(), Payload: []byte(`{"luid":"b","name":"Beta","project_luid":"project-1"}`)},
			}
			if err := store.UpsertResources(context.Background(), entries); err != nil {
				t.Fatal(err)
			}
			root := "content"
			if kind == "user" || kind == "group" {
				root = "admin"
			}
			var columns string
			for _, limit := range []string{"1", "", "25"} {
				args := []string{root, kind, "list", "--environment", "production", "--catalog"}
				if limit != "" {
					args = append(args, "--limit", limit)
				}
				var out strings.Builder
				if exit := app.Run(context.Background(), args, &out, options); exit != 0 {
					t.Fatalf("exit=%d %s", exit, out.String())
				}
				prefix := kind + "s["
				var header string
				for _, line := range strings.Split(out.String(), "\n") {
					if strings.HasPrefix(line, prefix) {
						header = line
						break
					}
				}
				start := strings.Index(header, "{")
				if start < 0 {
					t.Fatalf("limit=%q must keep tabular columns: %s", limit, out.String())
				}
				if columns == "" {
					columns = header[start:]
				} else if columns != header[start:] {
					t.Fatal(fmt.Sprintf("columns changed: %s vs %s", columns, header))
				}
			}
		})
	}
}
