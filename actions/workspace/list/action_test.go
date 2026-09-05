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
