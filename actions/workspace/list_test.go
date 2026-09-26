package workspace_test

import (
	"bytes"
	"context"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/output"
)

type listLister struct{}

func (listLister) List(context.Context, int, string) (workspaceaction.ListPage, error) {
	return workspaceaction.ListPage{Returned: 1, Total: 1, Limit: 20, Items: []workspaceaction.ListedWorkspace{{Workspace: workspaceaction.Workspace{Name: "development", ID: "ws_1", Root: "/var/tmp/tadx-tests/workspaces/development"}, Default: true, Available: true, ManifestValid: true}}}, nil
}

type listRecordingLister struct {
	page   workspaceaction.ListPage
	limit  int
	cursor string
}

func (l *listRecordingLister) List(_ context.Context, limit int, cursor string) (workspaceaction.ListPage, error) {
	l.limit, l.cursor = limit, cursor
	return l.page, nil
}

func TestListExecuteAllReturnsCompleteWorkspaceInventory(t *testing.T) {
	lister := &listRecordingLister{page: workspaceaction.ListPage{Returned: 2, Total: 2, Items: []workspaceaction.ListedWorkspace{{Workspace: workspaceaction.Workspace{Name: "alpha"}}, {Workspace: workspaceaction.Workspace{Name: "beta"}}}}}
	got, err := (&workspaceaction.Service{Lister: lister}).List(t.Context(), workspaceaction.ListInput{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if lister.limit != workspaceaction.ListMaxLimit || lister.cursor != "" || got.Page.Returned != 2 || got.Page.Total != 2 || got.Page.NextCursor != "" {
		t.Fatalf("all limit=%d cursor=%q page=%#v", lister.limit, lister.cursor, got.Page)
	}
}

func TestListExecuteAllRejectsIncompletePages(t *testing.T) {
	for _, page := range []workspaceaction.ListPage{{Returned: 1, Total: workspaceaction.ListMaxLimit + 1, Items: []workspaceaction.ListedWorkspace{{Workspace: workspaceaction.Workspace{Name: "alpha"}}}}, {Returned: 1, Total: 2, Items: []workspaceaction.ListedWorkspace{{Workspace: workspaceaction.Workspace{Name: "alpha"}}}}} {
		if _, err := (&workspaceaction.Service{Lister: &listRecordingLister{page: page}}).List(t.Context(), workspaceaction.ListInput{All: true}); err == nil {
			t.Fatalf("overflow or incomplete page %#v returned nil error", page)
		}
	}
}

func TestListExecuteReturnsBoundedCompactAndFullPage(t *testing.T) {
	result, err := (&workspaceaction.Service{Lister: listLister{}}).List(t.Context(), workspaceaction.ListInput{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(compact.Bytes(), []byte("ws_1")) || bytes.Contains(compact.Bytes(), []byte("/var/tmp/tadx-tests")) {
		t.Fatalf("compact output:\n%s", compact.String())
	}
	assertGolden(t, compact.Bytes(), "testdata/list/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(full.Bytes(), []byte("details:")) {
		t.Fatalf("full output:\n%s", full.String())
	}
	if !bytes.Contains(full.Bytes(), []byte("/var/tmp/tadx-tests/workspaces/development")) {
		t.Fatalf("full output omits the registered root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/list/output_full.toon")
}
