package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestDownloadDirIsPrivateDisposableWorkspaceState(t *testing.T) {
	root := t.TempDir()
	first, err := workspacecore.DownloadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := workspacecore.DownloadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Dir(first) != filepath.Join(root, ".tadx", "tmp") {
		t.Fatalf("download directories = %q, %q", first, second)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("download directory mode = %v", info.Mode().Perm())
	}
	if err := os.WriteFile(filepath.Join(first, "payload"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := workspacecore.Clean(context.Background(), root, "temporary"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("temporary cleanup left the download directory: %v", err)
	}
}

func TestDownloadDirRejectsSymlinkedControlState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation can require Windows developer mode")
	}
	for _, link := range []string{".tadx", filepath.Join(".tadx", "tmp")} {
		t.Run(link, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".tadx"), 0o700); err != nil {
				t.Fatal(err)
			}
			if link == ".tadx" {
				if err := os.Remove(filepath.Join(root, ".tadx")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(outside, filepath.Join(root, link)); err != nil {
				t.Fatal(err)
			}
			if _, err := workspacecore.DownloadDir(root); err == nil {
				t.Fatal("expected symlink boundary rejection")
			}
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Fatalf("download directory escaped the workspace: %v", entries)
			}
		})
	}
}
