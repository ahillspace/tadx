package move_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	workspacemove "github.com/ahillspace/tadx/actions/workspace/move"
	"github.com/ahillspace/tadx/internal/output"
)

type mover struct{ input workspacemove.Input }

func (m *mover) Move(_ context.Context, input workspacemove.Input) (workspacemove.Artifact, error) {
	m.input = input
	return workspacemove.Artifact{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", OldPath: "artifacts/workbook/Finance", State: "dirty", CurrentFingerprint: "sha256:new"}, nil
}

type cleanupWarningMover struct{}

func (cleanupWarningMover) Move(context.Context, workspacemove.Input) (workspacemove.Artifact, error) {
	warnings := make([]string, 25)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("cleanup warning %d", index)
	}
	return workspacemove.Artifact{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", State: "clean", Warnings: warnings}, nil
}

func TestExecuteMovesExactArtifactWithoutApply(t *testing.T) {
	dependency := &mover{}
	result, err := workspacemove.New(dependency).Execute(context.Background(), workspacemove.Input{SourceWorkspace: "one", DestinationWorkspace: "two", Kind: "workbook", LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if dependency.input.LUID != "wb-1" || len(result.Warnings) != 1 {
		t.Fatalf("result = %#v", result)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(compact.Bytes(), []byte("sha256:new")) || !bytes.Contains(compact.Bytes(), []byte("destination_workspace: two")) {
		t.Fatalf("compact output:\n%s", compact.String())
	}
	assertGolden(t, compact.Bytes(), "testdata/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, full.Bytes(), "testdata/output_full.toon")
}

func TestExecuteBoundsArtifactCleanupWarnings(t *testing.T) {
	result, err := workspacemove.New(cleanupWarningMover{}).Execute(context.Background(), workspacemove.Input{SourceWorkspace: "one", DestinationWorkspace: "two", Kind: "workbook", LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 20 || result.WarningsOmitted != 5 {
		t.Fatalf("warning bounds = %d, omitted = %d", len(result.Warnings), result.WarningsOmitted)
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rendered.Bytes(), []byte("warnings_omitted: 5")) {
		t.Fatalf("compact output:\n%s", rendered.String())
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
