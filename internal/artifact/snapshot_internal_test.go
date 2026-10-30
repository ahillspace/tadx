package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagedPublishSelectionHasNoSnapshotEntryLimit(t *testing.T) {
	root := createMoveTestWorkspace(t, "large-artifact")
	manager := NewWorkbookManager(time.Now)
	created, err := manager.Pull(t.Context(), WorkbookPull{Workspace: root, Filename: "Finance.twb", PayloadPath: PayloadFile(t, []byte("<workbook/>")), Metadata: moveTestMetadata()})
	if err != nil {
		t.Fatal(err)
	}
	for index := range 10001 {
		if err := os.WriteFile(filepath.Join(created.ArtifactPath, fmt.Sprintf("extra-%05d", index)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	relative, err := filepath.Rel(root, created.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := Resolve(t.Context(), root, Selector{Kind: "workbook", Path: filepath.ToSlash(relative)})
	if err != nil {
		t.Fatalf("managed publish selection rejected a large artifact tree: %v", err)
	}
	loaded, err := manager.Read(t.Context(), filepath.Join(root, filepath.FromSlash(selected.Path)))
	if err != nil || loaded.PayloadPath != created.CanonicalPath || selected.TreeFingerprint == "" {
		t.Fatalf("selected = %#v, payload = %#v, error = %v", selected, loaded, err)
	}
}

func TestArtifactTreeFingerprintDetectsContentAndPathChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "a"), 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "a", "inside")
	if err := os.WriteFile(file, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("sibling"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := artifactTreeFingerprint(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	again, err := artifactTreeFingerprint(t.Context(), root)
	if err != nil || again != before {
		t.Fatalf("unchanged tree hash = %q, want %q; error = %v", again, before, err)
	}
	if err := os.WriteFile(file, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := artifactTreeFingerprint(t.Context(), root)
	if err != nil || changed == before {
		t.Fatalf("content change was not detected: %q, error = %v", changed, err)
	}
	if err := os.Rename(file, filepath.Join(root, "a", "renamed")); err != nil {
		t.Fatal(err)
	}
	renamed, err := artifactTreeFingerprint(t.Context(), root)
	if err != nil || renamed == changed {
		t.Fatalf("path change was not detected: %q, error = %v", renamed, err)
	}
}

func TestArtifactTreeFingerprintHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := artifactTreeFingerprint(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v", err)
	}
	var calls int
	ctx, cancel = context.WithCancel(t.Context())
	reader := &snapshotContextReader{ctx: ctx, reader: snapshotReadFunc(func(buffer []byte) (int, error) {
		calls++
		cancel()
		return copy(buffer, "read"), nil
	})}
	if _, err := io.Copy(io.Discard, reader); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("stream cancellation error = %v, reads = %d", err, calls)
	}
}

type snapshotReadFunc func([]byte) (int, error)

func (f snapshotReadFunc) Read(buffer []byte) (int, error) { return f(buffer) }

func TestArtifactTreeFingerprintRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := artifactTreeFingerprint(t.Context(), root); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("linked snapshot error = %v", err)
	}
}
