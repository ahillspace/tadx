package register_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/ahillspace/tadx/actions/workspace/register"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type registrar struct {
	input   register.Input
	failure error
}

func (r *registrar) Register(_ context.Context, input register.Input) (register.Workspace, error) {
	r.input = input
	if r.failure != nil {
		return register.Workspace{}, r.failure
	}
	name := input.Name
	if name == "" {
		name = "adopted"
	}
	return register.Workspace{Name: name, ID: "ws_22222222222222222222222222222222", Root: "existing-root", ManifestVersion: 1, Registered: true}, nil
}

func TestExecuteAdoptsWorkspaceAndProjectsOutput(t *testing.T) {
	dependency := &registrar{}
	result, err := register.New(dependency).Execute(context.Background(), register.Input{Path: "existing-root", Name: "adopted"})
	if err != nil {
		t.Fatal(err)
	}
	if dependency.input.Path != "existing-root" || result.Status != "registered" || result.Workspace.Name != "adopted" {
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
	if !bytes.Contains(full.Bytes(), []byte("existing-root")) {
		t.Fatalf("full output omits the resolved machine-local root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/output_full.toon")
}

func TestExecuteRequiresPath(t *testing.T) {
	_, err := register.New(&registrar{}).Execute(context.Background(), register.Input{})
	assertUsage(t, err, "workspace.register.usage")
}

func TestExecuteSurfacesRegistrationFailure(t *testing.T) {
	dependency := &registrar{failure: errors.New("workspace manifest is incomplete")}
	_, err := register.New(dependency).Execute(context.Background(), register.Input{Path: "not-a-workspace"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "workspace.register.failed" || structured.Kind != errs.KindOperation {
		t.Fatalf("error = %#v", err)
	}
}

func assertUsage(t *testing.T, err error, id string) {
	t.Helper()
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != id || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
	if errs.ExitCode(err) != 2 {
		t.Fatalf("exit code = %d", errs.ExitCode(err))
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
