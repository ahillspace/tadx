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

func TestManagerCreatesAndResolvesNamedWorkspace(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("a", 16)))
	workspaceRoot := filepath.Join(root, "workspaces", "development")

	created, err := manager.Create(context.Background(), "Development", workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Development" || created.ID != "ws_61616161616161616161616161616161" {
		t.Fatalf("created = %#v", created)
	}
	for _, relative := range []string{"tadx.yaml", "artifacts", ".tadx"} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, relative)); err != nil {
			t.Fatalf("created entry %q: %v", relative, err)
		}
	}
	resolved, err := manager.Resolve(context.Background(), "development", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "Development" || resolved.ID != created.ID || resolved.Root != created.Root {
		t.Fatalf("resolved = %#v", resolved)
	}
}

func TestManagerRejectsCaseOnlyNameAndCanonicalRootCollisions(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("b", 32)))
	firstRoot := filepath.Join(root, "first")
	if _, err := manager.Create(context.Background(), "Development", firstRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(context.Background(), "development", filepath.Join(root, "second")); err == nil {
		t.Fatal("Create() accepted a case-only duplicate")
	}
	if _, err := manager.Create(context.Background(), "other", firstRoot); err == nil {
		t.Fatal("Create() accepted a duplicate canonical root")
	}
}

func TestManagerDoesNotResolveUnregisteredManifest(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(root, "unregistered")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "artifacts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "tadx.yaml"), []byte("version: 1\nworkspace:\n  id: ws_11111111111111111111111111111111\n  name: unregistered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := workspace.NewManager(configPath, nil)
	if _, err := manager.Resolve(context.Background(), "unregistered", ""); err == nil {
		t.Fatal("Resolve() accepted an unregistered manifest")
	}
}

func TestManagerResolvePrefersContainingRegisteredWorkspace(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("e", 16)+strings.Repeat("f", 16)))
	defaultRoot := filepath.Join(root, "default")
	containingRoot := filepath.Join(root, "containing")
	if _, err := manager.Create(context.Background(), "default", defaultRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(context.Background(), "containing", containingRoot); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(containingRoot, "nested", "directory")
	if err := os.MkdirAll(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(inside); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	resolved, err := manager.Resolve(context.Background(), "", "default")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "containing" {
		t.Fatalf("Resolve() name = %q, want containing", resolved.Name)
	}
}

func TestManagerCreateInitializesMissingUserConfiguration(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.yaml")
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("c", 16)))
	created, err := manager.Create(context.Background(), "development", filepath.Join(root, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "development" {
		t.Fatalf("created = %#v", created)
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != config.CurrentVersion || loaded.DefaultWorkspace != "development" {
		t.Fatalf("initialized config = %#v", loaded)
	}
}

func TestManagerCreateRejectsAnExistingEmptyRootWithNextStep(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(root, "existing-empty")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := workspace.NewManager(configPath, nil).Create(context.Background(), "development", workspaceRoot)
	if err == nil || !strings.Contains(err.Error(), "must not already exist") {
		t.Fatalf("error = %v", err)
	}
	entries, readErr := os.ReadDir(workspaceRoot)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("existing root changed: entries=%v error=%v", entries, readErr)
	}
}

func TestManagerCreateDoesNotReplaceMalformedUserConfiguration(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	malformed := []byte("version: [invalid\n")
	if err := os.WriteFile(configPath, malformed, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := workspace.NewManager(configPath, strings.NewReader(strings.Repeat("d", 16)))
	if _, err := manager.Create(context.Background(), "development", filepath.Join(root, "workspace")); err == nil {
		t.Fatal("Create() replaced malformed configuration")
	}
	current, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(malformed) {
		t.Fatalf("malformed configuration changed to %q", current)
	}
}

func TestManagerListUpgradesLegacyWorkspaceManifest(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	workspaceRoot := filepath.Join(root, "workspaces", "dev")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "artifacts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, ".tadx"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, config.WorkspaceConfigName), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyConfig := `version: 1
default_workspace: "` + filepath.ToSlash(workspaceRoot) + `"
`
	if err := os.WriteFile(configPath, []byte(legacyConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	page, err := workspace.NewManager(configPath, nil).List(context.Background(), 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "dev" || !page.Items[0].Available || !page.Items[0].ManifestValid {
		t.Fatalf("workspace page = %#v", page)
	}
	manifest, err := workspace.ReadManifest(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Workspace.Name != "dev" || manifest.Workspace.ID != page.Items[0].ID {
		t.Fatalf("upgraded manifest = %#v", manifest)
	}
}

func TestManagerDoesNotReplaceNonLegacyMalformedManifest(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	workspaceRoot := filepath.Join(root, "workspaces", "dev")
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(workspaceRoot, config.WorkspaceConfigName)
	malformed := []byte("version: 1\nunknown: preserve\n")
	if err := os.WriteFile(manifestPath, malformed, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyConfig := `version: 1
default_workspace: "` + filepath.ToSlash(workspaceRoot) + `"
`
	if err := os.WriteFile(configPath, []byte(legacyConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	page, err := workspace.NewManager(configPath, nil).List(context.Background(), 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ManifestValid {
		t.Fatalf("workspace page = %#v", page)
	}
	current, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(malformed) {
		t.Fatalf("malformed manifest changed to %q", current)
	}
}
