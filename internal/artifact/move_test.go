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

func TestMovePreservesIdentityBytesAndRelativePath(t *testing.T) {
	source := createWorkspace(t)
	destination := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	created, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: source, Filename: "Finance.twbx", Content: []byte("payload"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}

	moved, err := artifact.Move(context.Background(), artifact.MoveRequest{
		SourceWorkspace: source, DestinationWorkspace: destination, Selector: artifact.Selector{Kind: "workbook", LUID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if moved.LUID != "wb-1" || filepath.IsAbs(moved.Path) {
		t.Fatalf("Move() = %#v", moved)
	}
	if _, err := os.Stat(created.ArtifactPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(moved.CanonicalPath)))
	if err != nil || string(content) != "payload" {
		t.Fatalf("destination payload = %q, %v", content, err)
	}
}

func TestMoveRejectsDestinationCollision(t *testing.T) {
	source := createWorkspace(t)
	destination := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	for _, root := range []string{source, destination} {
		if _, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: root, Filename: "Finance.twb", Content: []byte("payload"), Metadata: validMetadata("Finance", "wb-1")}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := artifact.Move(context.Background(), artifact.MoveRequest{SourceWorkspace: source, DestinationWorkspace: destination, Selector: artifact.Selector{Kind: "workbook", LUID: "wb-1"}}); err == nil {
		t.Fatal("Move() accepted a destination collision")
	}
}
