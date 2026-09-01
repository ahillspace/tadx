package clone_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/ahillspace/tadx/actions/workspace/clone"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type cloner struct {
	input   clone.Input
	failure error
}

func (c *cloner) Clone(_ context.Context, input clone.Input) (clone.Workspace, error) {
	c.input = input
	if c.failure != nil {
		return clone.Workspace{}, c.failure
	}
	return clone.Workspace{Name: input.Name, ID: "ws_33333333333333333333333333333333", ManifestVersion: 1, Registered: true}, nil
}

func TestExecuteClonesWorkspaceAndProjectsOutput(t *testing.T) {
	dependency := &cloner{}
	result, err := clone.New(dependency).Execute(context.Background(), clone.Input{Source: "development", Name: "experiment", Path: "runtime-root"})
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
	assertGolden(t, compact.Bytes(), "testdata/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(full.Bytes(), []byte("runtime-root")) {
		t.Fatalf("full output leaks the machine-local root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/output_full.toon")
}

func TestExecuteRequiresSourceNameAndPath(t *testing.T) {
	for _, input := range []clone.Input{
		{Name: "experiment", Path: "root"},
		{Source: "development", Path: "root"},
		{Source: "development", Name: "experiment"},
	} {
		_, err := clone.New(&cloner{}).Execute(context.Background(), input)
		assertUsage(t, err, "workspace.clone.usage")
	}
}

func TestExecuteSurfacesCloneFailure(t *testing.T) {
	dependency := &cloner{failure: errors.New("destination artifact already exists")}
	_, err := clone.New(dependency).Execute(context.Background(), clone.Input{Source: "development", Name: "experiment", Path: "existing"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "workspace.clone.failed" || structured.Kind != errs.KindOperation {
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
	if !bytes.Equal(actual, bytes.TrimSuffix(expected, []byte("\n"))) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s", path, expected, actual)
	}
}
