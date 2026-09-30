package artifact

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWorkspaceRelativeErrorRendersPathsUnderTheWorkspaceRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "work space")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, "artifacts", "workbook", ".tadx-workbook-stage-1")
	backup := filepath.Join(root, "artifacts", "workbook", ".tadx-backup-Sales")
	pathErr := &fs.PathError{Op: "mkdir", Path: stage, Err: fs.ErrPermission}
	cause := fmt.Errorf("install workbook artifact: %w; restore previous workbook artifact from %q: %v", pathErr, backup, &os.LinkError{Op: "rename", Old: backup, New: filepath.Join(root, "artifacts", "workbook", "Sales"), Err: fs.ErrExist})
	err := withWorkspaceRelativePaths(cause, root)
	message := err.Error()
	for _, private := range []string{root, filepath.ToSlash(root), strings.Trim(strconv.Quote(root), `"`)} {
		if strings.Contains(message, private) {
			t.Fatalf("message %q still renders the workspace root %q", message, private)
		}
	}
	for _, want := range []string{"mkdir artifacts/workbook/.tadx-workbook-stage-1: permission denied", `"artifacts/workbook/.tadx-backup-Sales"`, "rename artifacts/workbook/.tadx-backup-Sales artifacts/workbook/Sales"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q lacks workspace-relative %q", message, want)
		}
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal("relative rendering broke errors.Is classification")
	}
	var unwrapped *fs.PathError
	if !errors.As(err, &unwrapped) || unwrapped.Path != stage {
		t.Fatalf("relative rendering broke errors.As: %#v", unwrapped)
	}
}

func TestWorkspaceRelativeErrorHandlesResolvedRootsAndLeavesOtherPaths(t *testing.T) {
	base := t.TempDir()
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "workspace")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(base), "elsewhere", "file.twbx")
	cause := fmt.Errorf("open %s: denied; root %s; outside %s", filepath.Join(resolved, "workspace", "tadx.yaml"), root, outside)
	message := withWorkspaceRelativePaths(cause, root).Error()
	if strings.Contains(message, filepath.Join(resolved, "workspace")) || strings.Contains(message, root) {
		t.Fatalf("message %q still renders a workspace root form", message)
	}
	if !strings.Contains(message, "open tadx.yaml: denied") || !strings.Contains(message, "root .;") {
		t.Fatalf("message %q lacks workspace-relative forms", message)
	}
	if !strings.Contains(message, outside) {
		t.Fatalf("message %q changed a path outside the workspace", message)
	}
	if withWorkspaceRelativePaths(nil, root) != nil {
		t.Fatal("nil error must stay nil")
	}
	plain := errors.New("no paths here")
	if got := withWorkspaceRelativePaths(plain, root); got != plain {
		t.Fatalf("an error without workspace paths should be returned unchanged, got %#v", got)
	}
}

func TestRelativizeWorkspacePathsHandlesWindowsSeparators(t *testing.T) {
	root := `D:\Data\TADX\workspaces\dev`
	message := `mkdir D:\Data\TADX\workspaces\dev\artifacts\workbook\.tadx-stage-1: Access is denied.; restore from "D:\\Data\\TADX\\workspaces\\dev\\artifacts\\workbook\\.tadx-backup-1"`
	got := relativizeWorkspacePaths(message, workspaceRootForms([]string{root}))
	want := `mkdir artifacts/workbook/.tadx-stage-1: Access is denied.; restore from "artifacts/workbook/.tadx-backup-1"`
	if got != want {
		t.Fatalf("relativizeWorkspacePaths() = %q, want %q", got, want)
	}
}
