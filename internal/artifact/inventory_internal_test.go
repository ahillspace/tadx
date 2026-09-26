package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidArtifactDiagnosticPrecedence(t *testing.T) {
	for _, test := range []struct {
		message string
		reason  string
		warning string
	}{
		{"metadata and canonical payload", "metadata_invalid", "Artifact metadata failed validation."},
		{"canonical payload and lineage", "canonical_payload_invalid", "Artifact canonical payload failed validation."},
		{"lineage and symbolic link", "lineage_invalid", "Artifact lineage sidecar failed validation."},
		{"symbolic link escapes", "containment_invalid", "Artifact containment failed validation."},
		{"other invalid structure", "structure_invalid", "Managed artifact structure failed validation."},
	} {
		t.Run(test.reason, func(t *testing.T) {
			err := errors.New(test.message)
			reason, warning := invalidArtifactDiagnostic(err)
			if got := reason; got != test.reason {
				t.Fatalf("reason = %q, want %q", got, test.reason)
			}
			if got := warning; got != test.warning {
				t.Fatalf("warning = %q, want %q", got, test.warning)
			}
		})
	}
}

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
