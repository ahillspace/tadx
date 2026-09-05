package create_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/ahillspace/tadx/actions/workspace/create"
	"github.com/ahillspace/tadx/internal/output"
)

type creator struct{ input create.Input }

func (c *creator) Create(_ context.Context, input create.Input) (create.Workspace, error) {
	c.input = input
	return create.Workspace{Name: input.Name, ID: "ws_11111111111111111111111111111111", Root: "/var/tmp/tadx-tests/workspaces/development", ManifestVersion: 1, Registered: true}, nil
}

func TestExecuteCreatesNamedWorkspaceAndProjectsOutput(t *testing.T) {
	dependency := &creator{}
	result, err := create.New(dependency).Execute(context.Background(), create.Input{Name: "development", Path: "runtime-root"})
	if err != nil {
		t.Fatal(err)
	}
	if dependency.input.Path != "runtime-root" || result.Workspace.Name != "development" {
		t.Fatalf("result = %#v, input = %#v", result, dependency.input)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, compact.Bytes(), "testdata/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full.Bytes(), []byte("/var/tmp/tadx-tests/workspaces/development")) {
		t.Fatalf("full output omits the registered root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/output_full.toon")
}

func TestExecuteAcceptsDefaultCreationPath(t *testing.T) {
	dependency := &creator{}
	if _, err := create.New(dependency).Execute(context.Background(), create.Input{Name: "development"}); err != nil {
		t.Fatal(err)
	}
	if dependency.input != (create.Input{Name: "development"}) {
		t.Fatalf("input = %#v", dependency.input)
	}
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
