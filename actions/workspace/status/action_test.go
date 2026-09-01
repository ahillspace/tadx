package status_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/ahillspace/tadx/actions/workspace/status"
	"github.com/ahillspace/tadx/internal/output"
)

type reader struct{}

func (reader) Status(context.Context, status.Input) (status.Workspace, status.Inventory, error) {
	return status.Workspace{Name: "development", ID: "ws_1"}, status.Inventory{
		Returned: 1, Limit: 20, ScanComplete: true, Dirty: 1,
		Items: []status.Artifact{{Kind: "workbook", LUID: "wb-1", Name: "Finance", Path: "artifacts/workbook/Finance", State: "dirty", CanonicalPath: "artifacts/workbook/Finance/Finance.twbx", BaselineFingerprint: "sha256:old", CurrentFingerprint: "sha256:new"}},
	}, nil
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
	if bytes.Contains(compact.Bytes(), []byte("sha256:new")) || !bytes.Contains(compact.Bytes(), []byte("dirty: 1")) || !bytes.Contains(compact.Bytes(), []byte("status: attention")) {
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
	assertGolden(t, full.Bytes(), "testdata/output_full.toon")
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
