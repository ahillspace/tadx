package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloneRejectsTombstoneRetainedByRealDeleteRecovery(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "rejected deletion and failed restoration"
		if committed {
			name = "completed deletion and failed cleanup"
		}
		t.Run(name, func(t *testing.T) {
			manager, configPath, source, _, created := moveSnapshotFixture(t)
			expected, err := Resolve(t.Context(), source.Root, Selector{Kind: "workbook", LUID: "wb-1"})
			if err != nil {
				t.Fatal(err)
			}
			var retained string
			restoreCalls, cleanupCalls := 0, 0
			operations := defaultDeleteOperations()
			operations.rename = func(from, to string) error {
				if from == created.ArtifactPath {
					retained = to
					if !committed {
						// A local edit after planning must make the real staged
						// snapshot comparison reject deletion and attempt restore.
						file, err := os.OpenFile(filepath.Join(from, "view.md"), os.O_WRONLY|os.O_APPEND, 0)
						if err != nil {
							return err
						}
						_, writeErr := file.WriteString("\nretained local sidecar edit\n")
						if err := errors.Join(writeErr, file.Close()); err != nil {
							return err
						}
					}
					return os.Rename(from, to)
				}
				if from == retained {
					restoreCalls++
					return errors.New("source restoration unavailable")
				}
				return os.Rename(from, to)
			}
			if committed {
				operations.removeAll = func(path string) error {
					if path == retained {
						cleanupCalls++
						return errors.New("deletion cleanup unavailable")
					}
					return os.RemoveAll(path)
				}
			}
			deleted, err := deleteWithOperations(t.Context(), DeleteRequest{Workspace: source.Root, ExpectedWorkspaceID: source.ID, Expected: expected}, operations)
			if committed && (err != nil || len(deleted.Warnings) != 1 || cleanupCalls != 1 || restoreCalls != 0) || !committed && (err == nil || !strings.Contains(err.Error(), "restoration failed") || restoreCalls != 1 || cleanupCalls != 0) {
				t.Fatalf("unexpected production delete boundary: committed=%t output=%+v error=%v", committed, deleted, err)
			}
			if retained == "" || !strings.HasPrefix(filepath.Base(retained), ".tadx-delete-") {
				t.Fatalf("delete did not retain its actual tombstone: %q", retained)
			}
			assertRecoveryEvidenceBlocksClone(t, manager, configPath, source, retained)
			if _, err := os.Lstat(created.ArtifactPath); !os.IsNotExist(err) {
				t.Fatalf("snapshot preparation inferred deletion outcome and restored the tombstone: %v", err)
			}
		})
	}
}
