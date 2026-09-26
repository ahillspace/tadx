package workspace_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/output"
)

type createCreator struct{ input workspaceaction.CreateInput }

func (c *createCreator) Create(_ context.Context, input workspaceaction.CreateInput) (workspaceaction.Registration, error) {
	c.input = input
	return workspaceaction.Registration{Workspace: workspaceaction.Workspace{Name: input.Name, ID: "ws_11111111111111111111111111111111", Root: "/var/tmp/tadx-tests/workspaces/development"}, ManifestVersion: 1, Registered: true}, nil
}

func TestCreateExecuteCreatesNamedWorkspaceAndProjectsOutput(t *testing.T) {
	dependency := &createCreator{}
	result, err := (&workspaceaction.Service{Creator: dependency}).Create(t.Context(), workspaceaction.CreateInput{Name: "development", Path: "runtime-root"})
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
	assertGolden(t, compact.Bytes(), "testdata/create/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full.Bytes(), []byte("/var/tmp/tadx-tests/workspaces/development")) {
		t.Fatalf("full output omits the registered root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/create/output_full.toon")
}

func TestCreateExecuteAcceptsDefaultCreationPath(t *testing.T) {
	dependency := &createCreator{}
	if _, err := (&workspaceaction.Service{Creator: dependency}).Create(t.Context(), workspaceaction.CreateInput{Name: "development"}); err != nil {
		t.Fatal(err)
	}
	if dependency.input != (workspaceaction.CreateInput{Name: "development"}) {
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
