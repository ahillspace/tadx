package artifact_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestMetadataSelectionKeepsFullPayloadValidationWithItsConsumer(t *testing.T) {
	root := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	created, err := manager.Pull(t.Context(), artifact.WorkbookPull{Workspace: root, Filename: "Finance.twb", PayloadPath: artifact.PayloadFile(t, []byte("<workbook/>")), Metadata: validMetadata("Finance", "book-1")})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := workspacecore.CanonicalRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(canonical, created.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created.CanonicalPath, []byte("<workbook changed='true'/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, selector := range []artifact.Selector{{Path: filepath.ToSlash(relative), Kind: "workbook"}, {Kind: "workbook", LUID: "book-1"}, {Kind: "workbook", Name: "Finance"}} {
		selected, err := artifact.Select(t.Context(), root, selector)
		if err != nil || selected.LUID != "book-1" || selected.Path != filepath.ToSlash(relative) || selected.State != "" || selected.TreeFingerprint != "" || selected.CurrentFingerprint != "" {
			t.Fatalf("selection invented validated payload state: %#v %v", selected, err)
		}
		resolved, err := artifact.Resolve(t.Context(), root, selector)
		if err != nil || resolved.State != artifact.StateDirty || resolved.CurrentFingerprint == "" || resolved.TreeFingerprint == "" || resolved.CurrentFingerprint == resolved.BaselineFingerprint {
			t.Fatalf("full resolution lost dirty-state verification: %#v %v", resolved, err)
		}
	}
	if err := os.Remove(created.CanonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(created.CanonicalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Select(t.Context(), root, artifact.Selector{Path: filepath.ToSlash(relative)}); err == nil {
		t.Fatal("metadata selection accepted a canonical directory")
	}
}

func TestNativePathValidationDoesNotReplaceFullContentValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Invalid.twb")
	if err := os.WriteFile(path, []byte("not a workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := artifact.ValidateNativePath(t.Context(), path, "workbook"); err != nil {
		t.Fatalf("valid file selection rejected: %v", err)
	}
	if _, err := artifact.ReadNative(t.Context(), path, "workbook"); err == nil {
		t.Fatal("full validation accepted malformed content")
	}
	for _, invalid := range []string{filepath.Dir(path), path + ".missing", filepath.Join(t.TempDir(), "Missing.twb")} {
		if err := artifact.ValidateNativePath(t.Context(), invalid, "workbook"); err == nil {
			t.Fatalf("invalid native selection accepted: %s", filepath.Base(invalid))
		}
	}
}
