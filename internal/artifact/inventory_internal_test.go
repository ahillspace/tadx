package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryFailsExplicitlyWhenBoundedScanCannotReachAllArtifacts(t *testing.T) {
	root := createMoveTestWorkspace(t, "bounded-inventory")
	for _, name := range []string{"first", "second"} {
		if err := os.MkdirAll(filepath.Join(root, "artifacts", "workbook", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inventoryWithScanLimit(context.Background(), root, InventoryOptions{Limit: 1}, 1); err == nil {
		t.Fatal("Inventory() presented an unusable continuation after reaching the scan limit")
	}
}
