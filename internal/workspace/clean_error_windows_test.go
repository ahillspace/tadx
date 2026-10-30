package workspace_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
	"golang.org/x/sys/windows"
)

func TestCleanAndPreviewMeasurementFailuresRenderRelativePaths(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, ".tadx", "logs")
	file := filepath.Join(directory, "keep.txt")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		t.Fatal(err)
	}
	held, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(held)
	for _, operation := range []struct {
		name string
		run  func(context.Context, string, string) (workspacecore.CleanResult, error)
	}{
		{"preview", workspacecore.PreviewClean},
		{"clean", workspacecore.Clean},
	} {
		t.Run(operation.name, func(t *testing.T) {
			_, err := operation.run(t.Context(), root, "logs")
			if err == nil || strings.Contains(err.Error(), root) || !strings.Contains(err.Error(), ".tadx/logs") {
				t.Fatalf("unsafe or incomplete measurement error: %v", err)
			}
			if _, ok := errors.AsType[*fs.PathError](err); !ok || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
				t.Fatalf("relative rendering lost the original measurement cause: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, ".tadx", "tmp.lock")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("measurement failure created temporary cleanup state: %v", err)
			}
			if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
				t.Fatalf("measurement failure changed disposable files: %q, %v", data, err)
			}
		})
	}
}

func TestCleanRemovalFailureRendersRelativePathsAndReleasesLock(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, ".tadx", "tmp", "held.txt")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("keep until the handle closes"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(file)
	if err != nil {
		t.Fatal(err)
	}
	// Denying delete sharing reproduces a real removal failure after the
	// cleanup lock has been acquired and the temporary tree measured.
	held, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if held != windows.InvalidHandle {
			_ = windows.CloseHandle(held)
		}
	})
	_, err = workspacecore.Clean(t.Context(), root, "temporary")
	if err == nil || strings.Contains(err.Error(), root) || !strings.Contains(err.Error(), ".tadx/tmp/held.txt") {
		t.Fatalf("unsafe or incomplete removal error: %v", err)
	}
	cause, ok := errors.AsType[*fs.PathError](err)
	if !ok || cause.Path != file || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("relative rendering lost the original sharing violation: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("failed cleanup removed the held file: %v", err)
	}
	if err := windows.CloseHandle(held); err != nil {
		t.Fatal(err)
	}
	held = windows.InvalidHandle
	if _, err := workspacecore.Clean(t.Context(), root, "temporary"); err != nil {
		t.Fatalf("cleanup lock was not released after failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tadx", "tmp")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("retry did not remove temporary state: %v", err)
	}
}
