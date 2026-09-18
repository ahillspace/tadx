package list_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	workspacelist "github.com/ahillspace/tadx/actions/workspace/list"
	"github.com/ahillspace/tadx/internal/output"
)

type lister struct{}

func (lister) List(context.Context, int, string) (workspacelist.Page, error) {
	return workspacelist.Page{Returned: 1, Total: 1, Limit: 20, Items: []workspacelist.Workspace{{Name: "development", ID: "ws_1", Root: "/var/tmp/tadx-tests/workspaces/development", Default: true, Available: true, ManifestValid: true}}}, nil
}

type recordingLister struct {
	page   workspacelist.Page
	limit  int
	cursor string
}

func (l *recordingLister) List(_ context.Context, limit int, cursor string) (workspacelist.Page, error) {
	l.limit, l.cursor = limit, cursor
	return l.page, nil
}

func TestExecuteAllReturnsCompleteWorkspaceInventory(t *testing.T) {
	lister := &recordingLister{page: workspacelist.Page{Returned: 2, Total: 2, Items: []workspacelist.Workspace{{Name: "alpha"}, {Name: "beta"}}}}
	got, err := workspacelist.New(lister).Execute(context.Background(), workspacelist.Input{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if lister.limit != workspacelist.MaxLimit || lister.cursor != "" || got.Page.Returned != 2 || got.Page.Total != 2 || got.Page.NextCursor != "" {
		t.Fatalf("all limit=%d cursor=%q page=%#v", lister.limit, lister.cursor, got.Page)
	}
}

func TestExecuteAllRejectsPaginationOverridesAndIncompletePages(t *testing.T) {
	for _, input := range []workspacelist.Input{{All: true, Limit: 1}, {All: true, Cursor: "0"}} {
		if _, err := workspacelist.New(&recordingLister{}).Execute(context.Background(), input); err == nil {
			t.Fatalf("Execute(%#v) error = nil", input)
		}
	}
	for _, page := range []workspacelist.Page{{Returned: 1, Total: workspacelist.MaxLimit + 1, Items: []workspacelist.Workspace{{Name: "alpha"}}}, {Returned: 1, Total: 2, Items: []workspacelist.Workspace{{Name: "alpha"}}}} {
		if _, err := workspacelist.New(&recordingLister{page: page}).Execute(context.Background(), workspacelist.Input{All: true}); err == nil {
			t.Fatalf("overflow or incomplete page %#v returned nil error", page)
		}
	}
}

func TestExecuteReturnsBoundedCompactAndFullPage(t *testing.T) {
	result, err := workspacelist.New(lister{}).Execute(context.Background(), workspacelist.Input{Limit: 20})
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
	assertGolden(t, compact.Bytes(), "testdata/output.toon")
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
	assertGolden(t, full.Bytes(), "testdata/output_full.toon")
}

func assertGolden(t *testing.T, actual []byte, path string) {
	t.Helper()
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s", path, expected, actual)
	}
}
