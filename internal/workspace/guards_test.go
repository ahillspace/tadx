package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceDeletionInspectionTreatsUnmanagedFilesAsDirty(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"artifacts", ".tadx"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty, err := workspacecore.HasUnmanagedEntries(context.Background(), root, nil)
	if err != nil || dirty {
		t.Fatalf("clean workspace: dirty=%t err=%v", dirty, err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty, err = workspacecore.HasUnmanagedEntries(context.Background(), root, nil)
	if err != nil || !dirty {
		t.Fatalf("unmanaged file: dirty=%t err=%v", dirty, err)
	}
}
