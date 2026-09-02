package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestCleanDisposableStatePreservesManagedArtifactsAndManifest(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"tadx.yaml":                  "version: 1\n",
		"artifacts/workbook/wb/data": "managed",
		".tadx/tmp/request/body":     "1234",
		".tadx/staging/pending":      "12",
		".tadx/cache/catalog":        "cache",
		".tadx/logs/debug.log":       "log",
	} {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := workspacecore.Clean(context.Background(), root, "temporary")
	if err != nil {
		t.Fatal(err)
	}
	if result.EntriesRemoved != 5 || result.BytesRemoved != 6 || len(result.Removed) != 2 {
		t.Fatalf("result = %#v", result)
	}
	for _, path := range []string{"tadx.yaml", "artifacts/workbook/wb/data", ".tadx/cache/catalog", ".tadx/logs/debug.log"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("preserved path %s: %v", path, err)
		}
	}
	for _, path := range []string{".tadx/tmp", ".tadx/staging"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatalf("disposable path %s remains: %v", path, err)
		}
	}
}

func TestCleanRejectsSymlinkedControlDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation can require Windows developer mode")
	}
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".tadx")); err != nil {
		t.Fatal(err)
	}
	if _, err := workspacecore.Clean(context.Background(), root, "all"); err == nil {
		t.Fatal("expected symlink boundary rejection")
	}
}

func TestCleanRejectsUnknownClass(t *testing.T) {
	if _, err := workspacecore.Clean(context.Background(), t.TempDir(), "artifacts"); err == nil {
		t.Fatal("expected unknown class rejection")
	}
}
