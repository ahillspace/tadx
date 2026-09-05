package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/readsource"
)

func TestUnfilteredLiveWorkbookListRefreshesSnapshotAndContinuesWithoutTableau(t *testing.T) {
	var signins atomic.Int32
	var inventoryReads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			signins.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			inventoryReads.Add(1)
			writer.Header().Set("X-Tableau-Request-Id", "projects-request")
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="project-root" name="Department"/><project id="project-ops" name="Ops" parentProjectId="project-root"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks" && request.URL.Query().Get("filter") == "":
			inventoryReads.Add(1)
			writer.Header().Set("X-Tableau-Request-Id", "workbooks-request")
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><workbooks><workbook id="workbook-b" name="Beta" updatedAt="2026-09-02T00:00:00Z"><project id="project-ops"/><owner id="user-1"/></workbook><workbook id="workbook-a" name="Alpha" contentUrl="alpha" description="Alpha reporting" createdAt="2026-08-01T00:00:00Z" updatedAt="2026-09-01T00:00:00Z"><project id="project-ops"/><owner id="user-1"/><tags><tag label="daily"/></tags></workbook></workbooks></tsResponse>`)
		default:
			http.Error(writer, fmt.Sprintf("unexpected %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery), http.StatusNotFound)
		}
	}))
	defer server.Close()

	runtime := inventoryListRuntime(t, server)
	commands := newRemoteContentCommands(runtime)
	first, err := commands.ListWorkbooks(context.Background(), workbooklist.Input{Environment: "production", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if signins.Load() != 1 || inventoryReads.Load() != 2 {
		t.Fatalf("sign-ins = %d, inventory reads = %d", signins.Load(), inventoryReads.Load())
	}
	if first.Page.Total != 2 || first.Page.Returned != 1 || first.Page.NextCursor == "" || first.Workbooks[0].LUID != "workbook-a" || first.Workbooks[0].ProjectPath != "Department/Ops" {
		t.Fatalf("first page = %#v", first)
	}
	if item := first.Workbooks[0]; item.ContentURL != "alpha" || item.Description != "Alpha reporting" || item.CreatedAt != "2026-08-01T00:00:00Z" || len(item.Tags) != 1 || item.Tags[0] != "daily" {
		t.Fatalf("complete workbook projection = %#v", item)
	}
	if first.Source == nil || first.Source.Mode != readsource.Tableau || !first.Source.CatalogRefreshed || first.Source.CatalogGeneration == "" || !containsString(first.Help, inventoryRefreshHelp) {
		t.Fatalf("source/help = %#v / %#v", first.Source, first.Help)
	}

	second, err := commands.ListWorkbooks(context.Background(), workbooklist.Input{Environment: "production", Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if signins.Load() != 1 || inventoryReads.Load() != 2 || len(second.Workbooks) != 1 || second.Workbooks[0].LUID != "workbook-b" || second.Source == nil || second.Source.Mode != readsource.Catalog {
		t.Fatalf("continuation = %#v; sign-ins = %d; reads = %d", second, signins.Load(), inventoryReads.Load())
	}

	store := catalog.NewStore(filepath.Dir(runtime.configPath), runtime.now)
	_, err = store.ReplaceResourceScope(context.Background(), catalog.ResourceScopeReplacement{
		Environment: "production", Site: "team-site", Kind: "workbook", Source: "tableau-rest", GeneratedAt: runtime.now().Add(time.Minute),
		Entries: []catalog.ResourceEntry{{LUID: "workbook-new", Name: "New"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = commands.ListWorkbooks(context.Background(), workbooklist.Input{Environment: "production", Limit: 1, Cursor: first.Page.NextCursor})
	if err == nil || !strings.Contains(err.Error(), "continuation cursor no longer identifies") {
		t.Fatalf("replaced snapshot continuation error = %v", err)
	}
	if signins.Load() != 1 || inventoryReads.Load() != 2 {
		t.Fatalf("failed continuation contacted Tableau: sign-ins = %d, reads = %d", signins.Load(), inventoryReads.Load())
	}
}

func inventoryListRuntime(t *testing.T, server *httptest.Server) *runtimeDependencies {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: team-site\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	return &runtimeDependencies{configPath: configPath, httpClient: server.Client(), now: func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) }, correlationID: "inventory-test"}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
