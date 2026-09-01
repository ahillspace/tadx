package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeleteRejectsMetadataChangeThatMakesArtifactDirty(t *testing.T) {
	root := createMoveTestWorkspace(t, "delete-revalidate")
	created, err := NewWorkbookManager(time.Now).Pull(context.Background(), WorkbookPull{
		Workspace: root, Filename: "Finance.twbx", Content: []byte("payload"), Metadata: moveTestMetadata(),
	})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := Resolve(context.Background(), root, Selector{Kind: "workbook", LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(created.ArtifactPath, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["local_baseline_fingerprint"] = "sha256:" + strings.Repeat("0", 64)
	changed, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(context.Background(), DeleteRequest{Workspace: root, Expected: expected}); err == nil {
		t.Fatal("Delete() accepted changed metadata")
	}
	if _, err := os.Stat(created.ArtifactPath); err != nil {
		t.Fatalf("artifact was removed after rejected delete: %v", err)
	}
}

func TestMoveRejectsSidecarChangeAtAtomicCommit(t *testing.T) {
	source := createMoveTestWorkspace(t, "move-source")
	destination := createMoveTestWorkspace(t, "move-destination")
	created, err := NewWorkbookManager(time.Now).Pull(context.Background(), WorkbookPull{
		Workspace: source, Filename: "Finance.twbx", Content: []byte("payload"), Metadata: moveTestMetadata(),
	})
	if err != nil {
		t.Fatal(err)
	}
	operations := defaultMoveOperations()
	operations.rename = func(from, to string) error {
		if sameFilesystemPath(from, created.ArtifactPath) {
			if err := os.WriteFile(filepath.Join(from, "view.md"), []byte("concurrent local edit"), 0o600); err != nil {
				return err
			}
		}
		return os.Rename(from, to)
	}
	if _, err := moveWithOperations(context.Background(), MoveRequest{SourceWorkspace: source, DestinationWorkspace: destination, Selector: Selector{Kind: "workbook", LUID: "wb-1"}}, operations); err == nil {
		t.Fatal("Move() accepted a sidecar change at commit")
	}
	if _, err := os.Stat(created.ArtifactPath); err != nil {
		t.Fatalf("source was not restored: %v", err)
	}
}

func TestMoveKeepsInstalledDestinationWhenSourceTombstoneCleanupPartiallyFails(t *testing.T) {
	source := createMoveTestWorkspace(t, "source")
	destination := createMoveTestWorkspace(t, "destination")
	created, err := NewWorkbookManager(time.Now).Pull(context.Background(), WorkbookPull{
		Workspace: source, Filename: "Finance.twbx", Content: []byte("payload"), Metadata: moveTestMetadata(),
	})
	if err != nil {
		t.Fatal(err)
	}
	operations := defaultMoveOperations()
	operations.removeAll = func(path string) error {
		if strings.Contains(filepath.Base(path), ".tadx-move-source-") {
			_ = os.Remove(filepath.Join(path, filepath.Base(created.CanonicalPath)))
			return errors.New("simulated partial tombstone cleanup failure")
		}
		return os.RemoveAll(path)
	}

	moved, err := moveWithOperations(context.Background(), MoveRequest{SourceWorkspace: source, DestinationWorkspace: destination, Selector: Selector{Kind: "workbook", LUID: "wb-1"}}, operations)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved.Warnings) != 1 || !strings.Contains(moved.Warnings[0], "cleanup") {
		t.Fatalf("Move() warnings = %#v", moved.Warnings)
	}
	content, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(moved.CanonicalPath)))
	if err != nil || string(content) != "payload" {
		t.Fatalf("installed destination payload = %q, %v", content, err)
	}
	if _, err := os.Stat(created.ArtifactPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source remained active: %v", err)
	}
}

func TestDeleteDoesNotRestorePartiallyCleanedTombstone(t *testing.T) {
	root := createMoveTestWorkspace(t, "delete")
	created, err := NewWorkbookManager(time.Now).Pull(context.Background(), WorkbookPull{
		Workspace: root, Filename: "Finance.twbx", Content: []byte("payload"), Metadata: moveTestMetadata(),
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := Resolve(context.Background(), root, Selector{Kind: "workbook", LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	operations := defaultDeleteOperations()
	operations.removeAll = func(path string) error {
		_ = os.Remove(filepath.Join(path, filepath.Base(created.CanonicalPath)))
		return errors.New("simulated partial tombstone cleanup failure")
	}

	deleted, err := deleteWithOperations(context.Background(), DeleteRequest{Workspace: root, Expected: item}, operations)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Warnings) != 1 || !strings.Contains(deleted.Warnings[0], "cleanup") {
		t.Fatalf("Delete() warnings = %#v", deleted.Warnings)
	}
	if _, err := os.Stat(created.ArtifactPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partially cleaned artifact was restored active: %v", err)
	}
}

func createMoveTestWorkspace(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func moveTestMetadata() WorkbookMetadata {
	return WorkbookMetadata{
		Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1",
		SourceEnvironment: "development", SourceSite: "test-site", SourceProjectName: "Ops", SourceProjectID: "project-1",
	}
}
