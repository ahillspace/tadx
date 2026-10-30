package workspace_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestCleanLockFailureRendersRelativePathsAndPreservesCause(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	for _, relative := range []bool{false, true} {
		name := "absolute"
		if relative {
			name = "relative"
		}
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(directory, name)
			blocked := filepath.Join(root, ".tadx", "tmp.lock")
			if err := os.MkdirAll(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			selected := root
			if relative {
				var err error
				selected, err = filepath.Rel(directory, root)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := workspacecore.Clean(t.Context(), selected, "temporary")
			if err == nil || strings.Contains(err.Error(), root) || strings.Contains(err.Error(), filepath.ToSlash(root)) || !strings.Contains(err.Error(), ".tadx/tmp.lock") {
				t.Fatalf("unsafe or incomplete cleanup error: %v", err)
			}
			cause, ok := errors.AsType[*fs.PathError](err)
			if !ok || cause.Path != blocked || !errors.Is(err, cause.Err) {
				t.Fatalf("relative rendering lost the original filesystem cause: %v", err)
			}
		})
	}
}
