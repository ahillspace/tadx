package status_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/ahillspace/tadx/actions/workspace/status"
	"github.com/ahillspace/tadx/internal/output"
)

type reader struct{}

func (reader) Status(context.Context, status.Input) (status.Workspace, status.Inventory, error) {
	return status.Workspace{Name: "development", ID: "ws_1", Root: "/var/tmp/tadx-tests/workspaces/development"}, status.Inventory{
		Returned: 1, Total: 1, Limit: 20, ScanComplete: true, Dirty: 1,
		Items: []status.Artifact{{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", State: "dirty", CanonicalPath: "artifacts/workbook/Finance/Finance.twbx", BaselineFingerprint: "sha256:old", CurrentFingerprint: "sha256:new"}},
	}, nil
}

type emptyReader struct{}

func (emptyReader) Status(context.Context, status.Input) (status.Workspace, status.Inventory, error) {
	return status.Workspace{Name: "empty", ID: "ws-empty", Root: "/var/tmp/tadx-tests/workspaces/empty"}, status.Inventory{Limit: 20, ScanComplete: true, Items: []status.Artifact{}}, nil
}

type recordingReader struct {
	inventory status.Inventory
	input     status.Input
}

func (r *recordingReader) Status(_ context.Context, input status.Input) (status.Workspace, status.Inventory, error) {
	r.input = input
	return status.Workspace{Name: "development", ID: "ws_1", Root: "/var/tmp/tadx-tests/workspaces/development"}, r.inventory, nil
}

func TestExecuteAllReturnsCompleteArtifactInventory(t *testing.T) {
	reader := &recordingReader{inventory: status.Inventory{Returned: 2, Total: 2, ScanComplete: true, Items: []status.Artifact{{Name: "alpha"}, {Name: "beta"}}}}
	got, err := status.New(reader).Execute(context.Background(), status.Input{All: true, Workspace: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if reader.input.Limit != status.MaxLimit || reader.input.Cursor != "" || got.Inventory.Returned != 2 || got.Inventory.Total != 2 || got.Inventory.NextCursor != "" {
		t.Fatalf("all input=%#v inventory=%#v", reader.input, got.Inventory)
	}
}

func TestExecuteAllRejectsPaginationOverridesAndIncompletePages(t *testing.T) {
	for _, input := range []status.Input{{All: true, Limit: 1}, {All: true, Cursor: "0"}} {
		if _, err := status.New(&recordingReader{}).Execute(context.Background(), input); err == nil {
			t.Fatalf("Execute(%#v) error = nil", input)
		}
	}
	for _, inventory := range []status.Inventory{{Returned: 1, Total: status.MaxLimit + 1, ScanComplete: true, Items: []status.Artifact{{Name: "alpha"}}}, {Returned: 1, Total: 2, ScanComplete: true, Items: []status.Artifact{{Name: "alpha"}}}, {Returned: 1, Total: 1, ScanComplete: false, Items: []status.Artifact{{Name: "alpha"}}}} {
		if _, err := status.New(&recordingReader{inventory: inventory}).Execute(context.Background(), status.Input{All: true}); err == nil {
			t.Fatalf("overflow or incomplete inventory %#v returned nil error", inventory)
		}
	}
}

func TestCompactOutputRetainsCompleteEmptyInventory(t *testing.T) {
	out, err := status.New(emptyReader{}).Execute(t.Context(), status.Input{Workspace: "empty"})
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

func TestExecuteKeepsDetailsOutOfCompactStatus(t *testing.T) {
	result, err := status.New(reader{}).Execute(context.Background(), status.Input{Workspace: "development", Limit: 20})
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
	assertGolden(t, compact.Bytes(), "testdata/output.toon")
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
