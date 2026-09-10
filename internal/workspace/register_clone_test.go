package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/workspace"
)

func writeWorkspaceDir(t *testing.T, root, id, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".tadx"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := "version: 1\nworkspace:\n  id: " + id + "\n  name: " + name + "\n"
	if err := os.WriteFile(filepath.Join(root, "tadx.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newConfig(t *testing.T) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func TestRegisterAdoptsExistingManifestIdentity(t *testing.T) {
	configPath := newConfig(t)
	workspaceRoot := filepath.Join(t.TempDir(), "existing")
	const manifestID = "ws_abababababababababababababababab"
	writeWorkspaceDir(t, workspaceRoot, manifestID, "adopted")

	manager := workspace.NewManager(configPath, nil)
	registered, err := manager.Register(context.Background(), "", workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	// Register must adopt the on-disk identity, never mint a fresh one.
	if registered.ID != manifestID {
		t.Fatalf("registered ID = %q, want the manifest ID %q", registered.ID, manifestID)
	}
	if registered.Name != "adopted" {
		t.Fatalf("registered name = %q", registered.Name)
	}
	resolved, err := manager.Resolve(context.Background(), "adopted", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != manifestID {
		t.Fatalf("resolved ID = %q", resolved.ID)
	}
}

func TestRegisterHonorsNameOverride(t *testing.T) {
	configPath := newConfig(t)
	workspaceRoot := filepath.Join(t.TempDir(), "existing")
	writeWorkspaceDir(t, workspaceRoot, "ws_cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd", "manifest-name")

	manager := workspace.NewManager(configPath, nil)
	registered, err := manager.Register(context.Background(), "override", workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if registered.Name != "override" {
		t.Fatalf("registered name = %q, want override", registered.Name)
	}
}

func TestRegisterRejectsNonWorkspaceDirectory(t *testing.T) {
	configPath := newConfig(t)
	plainDir := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(plainDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manager := workspace.NewManager(configPath, nil)
	if _, err := manager.Register(context.Background(), "", plainDir); err == nil {
		t.Fatal("Register() adopted a directory with no tadx.yaml")
	} else if advice, ok := err.(interface{ CorrectiveAction() string }); !ok || !strings.Contains(advice.CorrectiveAction(), "workspace create") {
		t.Fatalf("error does not point at create: %v", err)
	}
}

func TestRegisterRejectsNameIdentityAndRootCollisions(t *testing.T) {
	configPath := newConfig(t)
	manager := workspace.NewManager(configPath, nil)

	firstRoot := filepath.Join(t.TempDir(), "first")
	writeWorkspaceDir(t, firstRoot, "ws_11111111111111111111111111111111", "one")
	if _, err := manager.Register(context.Background(), "one", firstRoot); err != nil {
		t.Fatal(err)
	}

	// Same logical name, different directory and identity -> name collision.
	nameClashRoot := filepath.Join(t.TempDir(), "name-clash")
	writeWorkspaceDir(t, nameClashRoot, "ws_22222222222222222222222222222222", "unused")
	if _, err := manager.Register(context.Background(), "one", nameClashRoot); err == nil {
		t.Fatal("Register() accepted a duplicate name")
	}

	// Same identity as an existing registration, different name and root.
	idClashRoot := filepath.Join(t.TempDir(), "id-clash")
	writeWorkspaceDir(t, idClashRoot, "ws_11111111111111111111111111111111", "two")
	if _, err := manager.Register(context.Background(), "two", idClashRoot); err == nil {
		t.Fatal("Register() accepted a duplicate identity")
	}

	// Re-registering the exact same root under a new name -> root collision.
	if _, err := manager.Register(context.Background(), "three", firstRoot); err == nil {
		t.Fatal("Register() accepted a duplicate root")
	}
}

func TestCloneCopiesArtifactsUnderFreshIdentity(t *testing.T) {
	configPath := newConfig(t)
	base := t.TempDir()
	sourceRoot := filepath.Join(base, "source")

	// Deterministic identities: source create consumes the first 16 bytes, the
	// clone the next 16.
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("a", 16)+strings.Repeat("b", 16)))
	source, err := manager.Create(context.Background(), "source", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(source.Root, "artifacts", "workbooks", "revenue", "payload.tds")
	if err := os.MkdirAll(filepath.Dir(payload), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, []byte("managed-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	cloneRoot := filepath.Join(base, "clone")
	cloned, err := manager.Clone(context.Background(), "source", "clone", cloneRoot)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.ID == source.ID {
		t.Fatalf("clone reused the source identity %q", cloned.ID)
	}
	copied, err := os.ReadFile(filepath.Join(cloned.Root, "artifacts", "workbooks", "revenue", "payload.tds"))
	if err != nil || string(copied) != "managed-bytes" {
		t.Fatalf("cloned artifact = %q, err = %v", copied, err)
	}
	info, err := os.Stat(filepath.Join(cloned.Root, ".tadx"))
	if err != nil || !info.IsDir() {
		t.Fatalf("clone .tadx = %v, err = %v", info, err)
	}
	// The clone's local state starts empty; nothing machine-specific carries over.
	entries, err := os.ReadDir(filepath.Join(cloned.Root, ".tadx"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("clone .tadx entries = %#v, err = %v", entries, err)
	}
	resolved, err := manager.Resolve(context.Background(), "clone", "")
	if err != nil || resolved.ID != cloned.ID {
		t.Fatalf("resolved clone = %#v, err = %v", resolved, err)
	}
}

func TestCloneRejectsExistingDestination(t *testing.T) {
	configPath := newConfig(t)
	base := t.TempDir()
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("a", 64)))
	if _, err := manager.Create(context.Background(), "source", filepath.Join(base, "source")); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(base, "occupied")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Clone(context.Background(), "source", "clone", destination); err == nil {
		t.Fatal("Clone() overwrote an existing destination")
	}
}

func TestCloneRejectsSymlinkedSourceEntry(t *testing.T) {
	configPath := newConfig(t)
	base := t.TempDir()
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("a", 64)))
	source, err := manager.Create(context.Background(), "source", filepath.Join(base, "source"))
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(base, "outside-secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(source.Root, "artifacts", "link.tds")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := manager.Clone(context.Background(), "source", "clone", filepath.Join(base, "clone")); err == nil {
		t.Fatal("Clone() followed a symbolic link out of the source workspace")
	} else if !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "clone")); !os.IsNotExist(err) {
		t.Fatalf("failed clone left a destination behind: %v", err)
	}
}

func TestCloneRollsBackRootWhenRegistrationFails(t *testing.T) {
	configPath := newConfig(t)
	base := t.TempDir()
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("a", 64)))
	if _, err := manager.Create(context.Background(), "source", filepath.Join(base, "source")); err != nil {
		t.Fatal(err)
	}
	// Cloning under the already-taken "source" name fails registration; the
	// staged destination root must be rolled back.
	cloneRoot := filepath.Join(base, "clone")
	if _, err := manager.Clone(context.Background(), "source", "source", cloneRoot); err == nil {
		t.Fatal("Clone() accepted a duplicate name")
	}
	if _, err := os.Stat(cloneRoot); !os.IsNotExist(err) {
		t.Fatalf("failed clone left its root behind: %v", err)
	}
}
