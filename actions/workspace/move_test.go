package workspace_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/output"
)

type moveMover struct{ input workspaceaction.MoveInput }

func (m *moveMover) Move(_ context.Context, input workspaceaction.MoveInput) (workspaceaction.MoveArtifact, error) {
	m.input = input
	return workspaceaction.MoveArtifact{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", OldPath: "artifacts/workbook/Finance", State: "dirty", CurrentFingerprint: "sha256:new"}, nil
}

type moveCleanupWarningMover struct{}

func (moveCleanupWarningMover) Move(context.Context, workspaceaction.MoveInput) (workspaceaction.MoveArtifact, error) {
	warnings := make([]string, 25)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("cleanup warning %d", index)
	}
	return workspaceaction.MoveArtifact{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", State: "clean", Warnings: warnings}, nil
}

func TestMoveExecuteMovesExactArtifactWithoutApply(t *testing.T) {
	dependency := &moveMover{}
	result, err := (&workspaceaction.Service{Mover: dependency}).Move(t.Context(), workspaceaction.MoveInput{SourceWorkspace: "one", DestinationWorkspace: "two", Kind: "workbook", LUID: "wb-1"})
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
	assertGolden(t, compact.Bytes(), "testdata/move/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, full.Bytes(), "testdata/move/output_full.toon")
}

func TestMoveExecuteBoundsArtifactCleanupWarnings(t *testing.T) {
	result, err := (&workspaceaction.Service{Mover: moveCleanupWarningMover{}}).Move(t.Context(), workspaceaction.MoveInput{SourceWorkspace: "one", DestinationWorkspace: "two", Kind: "workbook", LUID: "wb-1"})
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
