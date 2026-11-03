//go:build windows

package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsWorkspaceLifecycleAndCloneSupportExtendedLengthPaths(t *testing.T) {
	base := t.TempDir()
	parent := base
	for len(parent) <= 300 {
		parent = filepath.Join(parent, "long-workspace-component")
	}
	root := filepath.Join(parent, "source")
	for _, path := range []string{filepath.Join(root, "artifacts"), filepath.Join(root, ".tadx")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tadx.yaml"), []byte("version: 1\nworkspace: {id: ws_01010101010101010101010101010101, name: source}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artifacts", "notes.txt"), []byte("long-path source content"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalRoot(root)
	if err != nil {
		t.Fatalf("canonical existing workspace with %d characters: %v", len(root), err)
	}
	want, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(canonical)
	if err != nil || !os.SameFile(want, got) {
		t.Fatalf("canonical root identifies a different directory: %v", err)
	}
	mutation, err := BeginMutation(t.Context(), root)
	if err != nil {
		t.Fatalf("long-path lifecycle entry: %v", err)
	}
	if err := mutation.Release(); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(base, "config.yaml"), nil)
	if _, err := manager.Register(t.Context(), "source", root); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "artifacts", "nested")
	if _, err := manager.PreviewClone(t.Context(), "source", "nested", nested); err == nil || !strings.Contains(err.Error(), "inside the source") {
		t.Fatalf("long-path nested clone preview: %v", err)
	}
	if _, err := manager.Clone(t.Context(), "source", "nested", nested); err == nil || !strings.Contains(err.Error(), "inside the source") {
		t.Fatalf("long-path nested clone execution: %v", err)
	}
	cloned, err := manager.Clone(t.Context(), "source", "copy", filepath.Join(parent, "copy"))
	if err != nil {
		t.Fatalf("long-path sibling clone: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(cloned.Root, "artifacts", "notes.txt")); err != nil || string(content) != "long-path source content" {
		t.Fatalf("long-path clone content: %q, %v", content, err)
	}
}
