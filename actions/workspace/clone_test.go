package workspace_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type cloneCloner struct {
	input   workspaceaction.CloneInput
	failure error
}

func (c *cloneCloner) Clone(_ context.Context, input workspaceaction.CloneInput) (workspaceaction.Registration, error) {
	c.input = input
	if c.failure != nil {
		return workspaceaction.Registration{}, c.failure
	}
	return workspaceaction.Registration{Workspace: workspaceaction.Workspace{Name: input.Name, ID: "ws_33333333333333333333333333333333", Root: "/var/tmp/tadx-tests/workspaces/experiment"}, ManifestVersion: 1, Registered: true}, nil
}

func TestCloneExecuteClonesWorkspaceAndProjectsOutput(t *testing.T) {
	dependency := &cloneCloner{}
	result, err := (&workspaceaction.Service{Cloner: dependency}).Clone(t.Context(), workspaceaction.CloneInput{Source: "development", Name: "experiment", Path: "runtime-root"})
	if err != nil {
		t.Fatal(err)
	}
	if dependency.input.Source != "development" || dependency.input.Path != "runtime-root" || result.Status != "cloned" || result.Workspace.Name != "experiment" {
		t.Fatalf("result = %#v, input = %#v", result, dependency.input)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, compact.Bytes(), "testdata/clone/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full.Bytes(), []byte("/var/tmp/tadx-tests/workspaces/experiment")) {
		t.Fatalf("full output omits the registered root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/clone/output_full.toon")
}

func TestCloneRequiresSource(t *testing.T) {
	for _, input := range []workspaceaction.CloneInput{
		{Name: "experiment", Path: "root"},
	} {
		_, err := (&workspaceaction.Service{Cloner: &cloneCloner{}}).Clone(t.Context(), input)
		cloneAssertUsage(t, err, "workspace.clone.usage")
	}
}

func TestCloneExecuteAcceptsDefaultClonePath(t *testing.T) {
	dependency := &cloneCloner{}
	if _, err := (&workspaceaction.Service{Cloner: dependency}).Clone(t.Context(), workspaceaction.CloneInput{Source: "development", Name: "experiment"}); err != nil {
		t.Fatal(err)
	}
	if dependency.input != (workspaceaction.CloneInput{Source: "development", Name: "experiment"}) {
		t.Fatalf("input = %#v", dependency.input)
	}
}

func TestCloneExecuteSurfacesCloneFailure(t *testing.T) {
	dependency := &cloneCloner{failure: errors.New("destination artifact already exists")}
	_, err := (&workspaceaction.Service{Cloner: dependency}).Clone(t.Context(), workspaceaction.CloneInput{Source: "development", Name: "experiment", Path: "existing"})
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.ID != "workspace.clone.failed" || structured.Kind != errs.KindOperation {
		t.Fatalf("error = %#v", err)
	}
}

func cloneAssertUsage(t *testing.T, err error, id string) {
	t.Helper()
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.ID != id || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	if errs.ExitCode(err) != 2 {
		t.Fatalf("exit code = %d", errs.ExitCode(err))
	}
}
