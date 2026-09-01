package artifact_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
)

func TestDeleteRevalidatesExactFingerprint(t *testing.T) {
	root := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	created, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: root, Filename: "Finance.twb", Content: []byte("payload"), Metadata: validMetadata("Finance", "wb-1")})
	if err != nil {
		t.Fatal(err)
	}
	item, err := artifact.Resolve(context.Background(), root, artifact.Selector{Kind: "workbook", LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created.CanonicalPath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Delete(context.Background(), artifact.DeleteRequest{Workspace: root, Expected: item}); err == nil {
		t.Fatal("Delete() accepted a changed fingerprint")
	}
	if _, err := os.Stat(created.ArtifactPath); err != nil {
		t.Fatalf("artifact changed after rejected delete: %v", err)
	}
}

func TestDeleteRemovesOnlyExactManagedArtifact(t *testing.T) {
	root := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	created, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: root, Filename: "Finance.twb", Content: []byte("payload"), Metadata: validMetadata("Finance", "wb-1")})
	if err != nil {
		t.Fatal(err)
	}
	item, err := artifact.Resolve(context.Background(), root, artifact.Selector{Path: itemRelative(t, root, created.ArtifactPath)})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := artifact.Delete(context.Background(), artifact.DeleteRequest{Workspace: root, Expected: item})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.LUID != "wb-1" {
		t.Fatalf("Delete() = %#v", deleted)
	}
	if _, err := os.Stat(created.ArtifactPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("artifact still exists: %v", err)
	}
}

func itemRelative(t *testing.T, root, target string) string {
	t.Helper()
	relative, err := filepath.Rel(root, target)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(relative)
}
