package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/output"
)

type statusReader struct{}

func (statusReader) Status(context.Context, workspaceaction.StatusInput) (workspaceaction.Workspace, workspaceaction.StatusInventory, error) {
	return workspaceaction.Workspace{Name: "development", ID: "ws_1", Root: "/var/tmp/tadx-tests/workspaces/development"}, workspaceaction.StatusInventory{
		Returned: 1, Total: 1, Limit: 20, ScanComplete: true, Dirty: 1,
		Items: []workspaceaction.StatusArtifact{{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", State: "dirty", CanonicalPath: "artifacts/workbook/Finance/Finance.twbx", BaselineFingerprint: "sha256:old", CurrentFingerprint: "sha256:new"}},
	}, nil
}

type statusEmptyReader struct{}

func (statusEmptyReader) Status(context.Context, workspaceaction.StatusInput) (workspaceaction.Workspace, workspaceaction.StatusInventory, error) {
	return workspaceaction.Workspace{Name: "empty", ID: "ws-empty", Root: "/var/tmp/tadx-tests/workspaces/empty"}, workspaceaction.StatusInventory{Limit: 20, ScanComplete: true, Items: []workspaceaction.StatusArtifact{}}, nil
}

type statusRecordingReader struct {
	inventory workspaceaction.StatusInventory
	input     workspaceaction.StatusInput
}

func (r *statusRecordingReader) Status(_ context.Context, input workspaceaction.StatusInput) (workspaceaction.Workspace, workspaceaction.StatusInventory, error) {
	r.input = input
	return workspaceaction.Workspace{Name: "development", ID: "ws_1", Root: "/var/tmp/tadx-tests/workspaces/development"}, r.inventory, nil
}

func TestStatusExecuteAllReturnsCompleteArtifactInventory(t *testing.T) {
	reader := &statusRecordingReader{inventory: workspaceaction.StatusInventory{Returned: 2, Total: 2, ScanComplete: true, Items: []workspaceaction.StatusArtifact{{Name: "alpha"}, {Name: "beta"}}}}
	got, err := (&workspaceaction.Service{Reader: reader}).Status(t.Context(), workspaceaction.StatusInput{All: true, Workspace: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if reader.input.Limit != workspaceaction.StatusMaxLimit || reader.input.Cursor != "" || got.Inventory.Returned != 2 || got.Inventory.Total != 2 || got.Inventory.NextCursor != "" {
		t.Fatalf("all input=%#v inventory=%#v", reader.input, got.Inventory)
	}
}

func TestStatusExecuteAllRejectsIncompletePages(t *testing.T) {
	for _, inventory := range []workspaceaction.StatusInventory{{Returned: 1, Total: workspaceaction.StatusMaxLimit + 1, ScanComplete: true, Items: []workspaceaction.StatusArtifact{{Name: "alpha"}}}, {Returned: 1, Total: 2, ScanComplete: true, Items: []workspaceaction.StatusArtifact{{Name: "alpha"}}}, {Returned: 1, Total: 1, ScanComplete: false, Items: []workspaceaction.StatusArtifact{{Name: "alpha"}}}} {
		if _, err := (&workspaceaction.Service{Reader: &statusRecordingReader{inventory: inventory}}).Status(t.Context(), workspaceaction.StatusInput{All: true}); err == nil {
			t.Fatalf("overflow or incomplete inventory %#v returned nil error", inventory)
		}
	}
}

func TestStatusCompactOutputRetainsCompleteEmptyInventory(t *testing.T) {
	out, err := (&workspaceaction.Service{Reader: statusEmptyReader{}}).Status(t.Context(), workspaceaction.StatusInput{Workspace: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(out.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	inventory := payload["artifacts"].(map[string]any)
	if inventory["total"] != float64(0) {
		t.Fatalf("total = %v, want 0", inventory["total"])
	}
	items, ok := inventory["artifacts"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("artifacts = %#v, want empty array", inventory["artifacts"])
	}
}

func TestStatusExecuteKeepsDetailsOutOfCompactStatus(t *testing.T) {
	result, err := (&workspaceaction.Service{Reader: statusReader{}}).Status(t.Context(), workspaceaction.StatusInput{Workspace: "development", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(compact.Bytes(), []byte("sha256:new")) || bytes.Contains(compact.Bytes(), []byte("/var/tmp/tadx-tests")) || !bytes.Contains(compact.Bytes(), []byte("dirty: 1")) || !bytes.Contains(compact.Bytes(), []byte("status: attention")) {
		t.Fatalf("compact output:\n%s", compact.String())
	}
	assertGolden(t, compact.Bytes(), "testdata/status/output.toon")
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, result, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full.Bytes(), []byte("\"sha256:new\"")) {
		t.Fatalf("full output:\n%s", full.String())
	}
	if !bytes.Contains(full.Bytes(), []byte("/var/tmp/tadx-tests/workspaces/development")) {
		t.Fatalf("full output omits the registered root:\n%s", full.String())
	}
	assertGolden(t, full.Bytes(), "testdata/status/output_full.toon")
}
