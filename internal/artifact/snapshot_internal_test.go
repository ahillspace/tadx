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

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
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
	canonical, err := workspacecore.CanonicalRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(canonical, created.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := Resolve(t.Context(), root, Selector{Kind: "workbook", Path: filepath.ToSlash(relative)})
	if err != nil {
		t.Fatalf("managed publish selection rejected a large artifact tree: %v", err)
	}
	loaded, err := manager.Read(t.Context(), filepath.Join(root, filepath.FromSlash(selected.Path)))
	if err != nil || selected.TreeFingerprint == "" {
		t.Fatalf("selected = %#v, payload = %#v, error = %v", selected, loaded, err)
	}
	loadedInfo, err := os.Stat(loaded.PayloadPath)
	if err != nil {
		t.Fatal(err)
	}
	createdInfo, err := os.Stat(created.CanonicalPath)
	if err != nil || !os.SameFile(loadedInfo, createdInfo) {
		t.Fatalf("selected payload identifies a different file: %q, want %q: %v", loaded.PayloadPath, created.CanonicalPath, err)
	}
}

func TestArtifactSnapshotUsesCanonicalFileIdentityOnCaseInsensitiveFilesystems(t *testing.T) {
	root := createMoveTestWorkspace(t, "case-alias")
	created, err := NewWorkbookManager(time.Now).Pull(t.Context(), WorkbookPull{Workspace: root, Filename: "Finance.twb", PayloadPath: PayloadFile(t, []byte("<workbook/>")), Metadata: moveTestMetadata()})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := readMetadata(created.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata.CanonicalPayload = strings.ToUpper(metadata.CanonicalPayload)
	if _, err := os.Stat(filepath.Join(created.ArtifactPath, metadata.CanonicalPayload)); os.IsNotExist(err) {
		t.Skip("filesystem does not support this case alias")
	}
	encoded, err := encodeWorkbookMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created.ArtifactPath, "metadata.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	selected, err := Resolve(t.Context(), root, Selector{Kind: "workbook", LUID: metadata.TableauID})
	if err != nil || selected.State != StateClean || selected.CurrentFingerprint != metadata.LocalBaselineFingerprint || selected.TreeFingerprint == "" {
		t.Fatalf("case-aliased native payload: %#v, %v", selected, err)
	}
}

func TestArtifactSnapshotDistinguishesMissingAndNonregularCanonicalPayload(t *testing.T) {
	for _, entry := range []string{"regular", "missing", "directory", "symlink", "outside tree"} {
		t.Run(entry, func(t *testing.T) {
			root := t.TempDir()
			canonical := filepath.Join(root, "Finance.twb")
			switch entry {
			case "regular":
				if err := os.WriteFile(canonical, []byte("<workbook/>"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(canonical, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside.twb")
				if err := os.WriteFile(outside, []byte("<workbook/>"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, canonical); err != nil {
					t.Skipf("symlink creation unavailable: %v", err)
				}
			case "outside tree":
				canonical = filepath.Join(t.TempDir(), "outside.twb")
				if err := os.WriteFile(canonical, []byte("<workbook/>"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			tree, current, err := artifactSnapshotFingerprints(t.Context(), root, canonical)
			switch entry {
			case "regular":
				if err != nil || tree == "" || current != fingerprint([]byte("<workbook/>")) {
					t.Fatalf("regular canonical snapshot = %q, %q, %v", tree, current, err)
				}
			case "missing":
				if !errors.Is(err, os.ErrNotExist) || tree == "" || current != "" {
					t.Fatalf("missing canonical snapshot = %q, %q, %v", tree, current, err)
				}
			case "directory":
				if err == nil || errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "regular file") {
					t.Fatalf("nonregular canonical snapshot error = %v", err)
				}
			case "symlink":
				if err == nil || errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "symbolic link") {
					t.Fatalf("linked canonical snapshot error = %v", err)
				}
			case "outside tree":
				if err == nil || errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unobserved existing canonical snapshot error = %v", err)
				}
			}
		})
	}
}

func TestResolveMissingCanonicalPayloadRetainsTreeSnapshot(t *testing.T) {
	root := createMoveTestWorkspace(t, "missing-payload")
	created, err := NewWorkbookManager(time.Now).Pull(t.Context(), WorkbookPull{Workspace: root, Filename: "Finance.twb", PayloadPath: PayloadFile(t, []byte("<workbook/>")), Metadata: moveTestMetadata()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(created.CanonicalPath); err != nil {
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
	for _, selector := range []Selector{{Kind: "workbook", LUID: moveTestMetadata().TableauID}, {Path: filepath.ToSlash(relative)}} {
		item, err := Resolve(t.Context(), root, selector)
		if err != nil || item.State != StateMissing || item.CurrentFingerprint != "" || item.TreeFingerprint == "" {
			t.Fatalf("missing payload selection = %#v, %v", item, err)
		}
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
	reader := &contextReader{ctx: ctx, reader: snapshotReadFunc(func(buffer []byte) (int, error) {
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
