package artifact

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestInvalidArtifactDiagnosticPrecedence(t *testing.T) {
	for _, test := range []struct {
		message string
		reason  string
		warning string
	}{
		{"metadata and canonical payload", "metadata_invalid", "Artifact metadata failed validation."},
		{"canonical payload and lineage", "canonical_payload_invalid", "Artifact canonical payload failed validation."},
		{"lineage and symbolic link", "lineage_invalid", "Artifact lineage sidecar failed validation."},
		{"symbolic link escapes", "containment_invalid", "Artifact containment failed validation."},
		{"other invalid structure", "structure_invalid", "Managed artifact structure failed validation."},
	} {
		t.Run(test.reason, func(t *testing.T) {
			err := errors.New(test.message)
			reason, warning := invalidArtifactDiagnostic(err)
			if got := reason; got != test.reason {
				t.Fatalf("reason = %q, want %q", got, test.reason)
			}
			if got := warning; got != test.warning {
				t.Fatalf("warning = %q, want %q", got, test.warning)
			}
		})
	}
}

func TestInventoryFailsExplicitlyWhenBoundedScanCannotReachAllArtifacts(t *testing.T) {
	root := createMoveTestWorkspace(t, "bounded-inventory")
	for _, name := range []string{"first", "second"} {
		if err := os.MkdirAll(filepath.Join(root, "artifacts", "workbook", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inventoryWithScanLimit(context.Background(), root, InventoryOptions{Limit: 1}, 1); err == nil {
		t.Fatal("Inventory() presented an unusable continuation after reaching the scan limit")
	}
}

func TestArtifactOperationsRejectLinkedManagedParents(t *testing.T) {
	for _, layout := range []struct{ kind, parent string }{
		{"workbook", "artifacts"},
		{"workbook", "artifacts/workbook"},
		{"lineage", "artifacts"},
		{"lineage", "artifacts/lineage"},
		{"lineage", "artifacts/lineage/flow"},
	} {
		for _, operation := range []string{"inventory", "resolve path", "resolve identity", "delete", "move source preview", "move source"} {
			t.Run(layout.kind+"/"+layout.parent+"/"+operation, func(t *testing.T) {
				outside := createMoveTestWorkspace(t, "outside")
				var relative string
				if layout.kind == "workbook" {
					pulled, err := NewWorkbookManager(time.Now).Pull(t.Context(), WorkbookPull{Workspace: outside, Filename: "Finance.twb", Content: []byte("owned payload"), Metadata: moveTestMetadata()})
					if err != nil {
						t.Fatal(err)
					}
					relative, err = filepath.Rel(outside, pulled.ArtifactPath)
					if err != nil {
						t.Fatal(err)
					}
					relative = filepath.ToSlash(relative)
				} else {
					relative = pullSampleLineage(t, outside).Path
				}
				expected, err := Resolve(t.Context(), outside, Selector{Path: relative})
				if err != nil {
					t.Fatal(err)
				}
				workspace := createMoveTestWorkspace(t, "workspace")
				link := filepath.Join(workspace, filepath.FromSlash(layout.parent))
				if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
					t.Fatal(err)
				}
				linkArtifactTestDirectory(t, link, filepath.Join(outside, filepath.FromSlash(layout.parent)))
				switch operation {
				case "inventory":
					_, err = Inventory(t.Context(), workspace, InventoryOptions{Limit: 20})
				case "resolve path":
					_, err = Resolve(t.Context(), workspace, Selector{Path: relative})
				case "resolve identity":
					_, err = Resolve(t.Context(), workspace, Selector{Kind: expected.Kind, LUID: expected.LUID})
				case "delete":
					_, err = Delete(t.Context(), DeleteRequest{Workspace: workspace, Expected: expected})
				case "move source preview", "move source":
					request := MoveRequest{SourceWorkspace: workspace, DestinationWorkspace: createMoveTestWorkspace(t, "destination"), Selector: Selector{Path: relative}}
					if operation == "move source preview" {
						_, err = PreviewMove(t.Context(), request)
					} else {
						_, err = Move(t.Context(), request)
					}
				}
				if err == nil {
					t.Errorf("%s accepted linked managed parent", operation)
				}
				if _, err := os.Stat(filepath.Join(outside, filepath.FromSlash(expected.CanonicalPath))); err != nil {
					t.Fatalf("outside artifact changed: %v", err)
				}
			})
		}
	}
}

func TestMoveRejectsLinkedDestinationParentsBeforeStaging(t *testing.T) {
	for _, parent := range []string{"artifacts", "artifacts/workbook"} {
		for _, preview := range []bool{true, false} {
			t.Run(parent+"/"+map[bool]string{true: "preview", false: "apply"}[preview], func(t *testing.T) {
				source := createMoveTestWorkspace(t, "source")
				pulled, err := NewWorkbookManager(time.Now).Pull(t.Context(), WorkbookPull{Workspace: source, Filename: "Finance.twb", Content: []byte("owned payload"), Metadata: moveTestMetadata()})
				if err != nil {
					t.Fatal(err)
				}
				destination := createMoveTestWorkspace(t, "destination")
				outside := t.TempDir()
				link := filepath.Join(destination, filepath.FromSlash(parent))
				if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
					t.Fatal(err)
				}
				linkArtifactTestDirectory(t, link, outside)
				request := MoveRequest{SourceWorkspace: source, DestinationWorkspace: destination, Selector: Selector{Kind: "workbook", LUID: "wb-1"}}
				if preview {
					_, err = PreviewMove(t.Context(), request)
				} else {
					_, err = Move(t.Context(), request)
				}
				if err == nil {
					t.Error("move accepted linked destination parent")
				}
				if _, err := os.Stat(pulled.CanonicalPath); err != nil {
					t.Fatalf("source artifact changed: %v", err)
				}
				entries, err := os.ReadDir(outside)
				if err != nil || len(entries) != 0 {
					t.Fatalf("outside destination changed: %v, %v", entries, err)
				}
			})
		}
	}
}

func TestManagedArtifactsAllowWorkspaceAncestorAliases(t *testing.T) {
	container := t.TempDir()
	root := filepath.Join(container, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkbookManager(time.Now).Pull(t.Context(), WorkbookPull{Workspace: root, Filename: "Finance.twb", Content: []byte("owned payload"), Metadata: moveTestMetadata()}); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	linkArtifactTestDirectory(t, alias, container)
	selected := filepath.Join(alias, "workspace")
	page, err := Inventory(t.Context(), selected, InventoryOptions{Limit: 20})
	if err != nil || page.Clean != 1 {
		t.Fatalf("ancestor alias rejected: %#v, %v", page, err)
	}
	if _, err := Resolve(t.Context(), selected, Selector{Kind: "workbook", LUID: "wb-1"}); err != nil {
		t.Fatal(err)
	}
}

func linkArtifactTestDirectory(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			t.Fatalf("create directory junction: %v: %s", err, output)
		}
	} else if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove owned directory link: %v", err)
		}
	})
}
