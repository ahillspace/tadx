package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/catalog"
)

func TestLegacyPartialSnapshotRemainsReadableThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("legacy snapshot contacted Tableau: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	runtime := inventoryListRuntime(t, server)
	store := catalog.NewStore(filepath.Dir(runtime.configPath), runtime.now)
	ctx := context.Background()
	id, err := store.SavePartialInventory(ctx, catalog.ResourceScopeReplacement{
		Environment: "production", Site: "team-site", Kind: "workbook", GeneratedAt: runtime.now(),
		Entries: []catalog.ResourceEntry{
			{Environment: "production", Site: "team-site", Kind: "workbook", LUID: "a", Name: "Alpha", Payload: []byte(`{"luid":"a","name":"Alpha"}`)},
			{Environment: "production", Site: "team-site", Kind: "workbook", LUID: "b", Name: "Beta", Payload: []byte(`{"luid":"b","name":"Beta"}`)},
		},
	}, "One legacy row was unavailable.")
	if err != nil {
		t.Fatal(err)
	}
	token := catalog.PartialInventoryCursor(id, catalog.ResourceQuery{Environment: "production", Site: "team-site", Kind: "workbook", Limit: 1, Offset: 1})
	// Use the existing action envelope, as an older producer would have done.
	first, err := workbooklist.New(legacySnapshotPage{token: token}).Execute(ctx, workbooklist.Input{Environment: "production", Site: "team-site", Limit: 1})
	if err != nil || first.Page.NextCursor == "" {
		t.Fatalf("legacy envelope = %#v, %v", first, err)
	}
	var out strings.Builder
	exit := Run(ctx, []string{"content", "workbook", "list", "--environment", "production", "--cursor", first.Page.NextCursor}, &out, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), Now: runtime.now})
	if exit != 0 || !strings.Contains(out.String(), "Beta") || !strings.Contains(out.String(), "coverage: partial") || strings.Contains(out.String(), "next_cursor:") {
		t.Fatalf("exit=%d output=%s", exit, out.String())
	}
}

type legacySnapshotPage struct{ token string }

func (p legacySnapshotPage) ListWorkbooks(context.Context, workbooklist.PageRequest) (workbooklist.Page, error) {
	return workbooklist.Page{Number: 1, Size: 1, Total: 2, Workbooks: []workbooklist.Workbook{{LUID: "a", Name: "Alpha"}}, SnapshotCursor: p.token}, nil
}
