package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestLoadMigratesSharedAbsoluteWorkspaceDefaults(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	workspaceRoot := filepath.Join(directory, "workspaces", "dev")
	contents := `version: 1
default_environment: dev
default_workspace: ` + quoteYAML(workspaceRoot) + `
environments:
  dev:
    url: https://example.test
    auth:
      type: pat
    default_workspace: ` + quoteYAML(workspaceRoot) + `
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	// Load migrates in memory but never writes: the on-disk file stays legacy
	// and no backup is produced until a locked Update persists the upgrade.
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultWorkspace != "dev" || loaded.Environments["dev"].DefaultWorkspace != "dev" {
		t.Fatalf("migrated defaults = %q, %q", loaded.DefaultWorkspace, loaded.Environments["dev"].DefaultWorkspace)
	}
	registration, ok := loaded.Workspaces["dev"]
	if !ok || registration.Path != workspaceRoot || !strings.HasPrefix(registration.ID, "ws_") {
		t.Fatalf("migrated workspace = %#v, present = %t", registration, ok)
	}
	if afterLoad, readErr := os.ReadFile(path); readErr != nil || string(afterLoad) != contents {
		t.Fatalf("Load persisted a migration: err=%v contents=\n%s", readErr, afterLoad)
	}
	if _, statErr := os.Stat(path + ".pre-workspace-migration-v1.bak"); !os.IsNotExist(statErr) {
		t.Fatalf("Load wrote a migration backup: %v", statErr)
	}

	// A locked Update persists the migration and preserves the legacy backup,
	// even when the mutator reports no logical change of its own.
	if _, err := config.Update(path, false, func(c config.Config) (config.Config, error) {
		return config.Config{}, config.ErrNoChange
	}); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), "default_workspace: "+quoteYAML(workspaceRoot)) {
		t.Fatalf("legacy defaults remain in persisted config:\n%s", persisted)
	}
	backup, err := os.ReadFile(path + ".pre-workspace-migration-v1.bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != contents {
		t.Fatalf("migration backup differs from original config:\n%s", backup)
	}
	// Once persisted, the workspace identity is stable across reloads.
	first, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.Workspaces["dev"].ID == "" || reloaded.Workspaces["dev"].ID != first.Workspaces["dev"].ID {
		t.Fatalf("workspace ID changed from %q to %q", first.Workspaces["dev"].ID, reloaded.Workspaces["dev"].ID)
	}
}

func TestUpdateDoesNotOverwriteDifferentWorkspaceMigrationBackup(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	workspaceRoot := filepath.Join(directory, "workspaces", "dev")
	contents := "version: 1\ndefault_workspace: " + quoteYAML(workspaceRoot) + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath := path + ".pre-workspace-migration-v1.bak"
	if err := os.WriteFile(backupPath, []byte("different prior configuration\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The conflicting-backup guard now lives on the write path: Load reads the
	// legacy config without touching the backup, and Update refuses to persist
	// the migration (and does not clobber the legacy config) when a differing
	// backup already exists.
	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load() over a differing backup should still read: %v", err)
	}
	_, err := config.Update(path, false, func(c config.Config) (config.Config, error) {
		return config.Config{}, config.ErrNoChange
	})
	if err == nil || !strings.Contains(err.Error(), "existing workspace migration backup differs") {
		t.Fatalf("Update() error = %v", err)
	}
	current, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(current) != contents {
		t.Fatalf("legacy config changed to %q", current)
	}
}

func TestLoadMigratesDistinctLegacyRootsWithSameLeafDeterministically(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	firstRoot := filepath.Join(directory, "first", "dev")
	secondRoot := filepath.Join(directory, "second", "dev")
	contents := `version: 1
default_workspace: ` + quoteYAML(firstRoot) + `
environments:
  first:
    url: https://first.example.test
    auth:
      type: pat
    default_workspace: ` + quoteYAML(firstRoot) + `
  second:
    url: https://second.example.test
    auth:
      type: pat
    default_workspace: ` + quoteYAML(secondRoot) + `
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	firstName := loaded.Environments["first"].DefaultWorkspace
	secondName := loaded.Environments["second"].DefaultWorkspace
	if firstName != "dev" || secondName == "" || strings.EqualFold(firstName, secondName) {
		t.Fatalf("migrated workspace names = %q, %q", firstName, secondName)
	}
	if loaded.DefaultWorkspace != firstName {
		t.Fatalf("global default = %q, want %q", loaded.DefaultWorkspace, firstName)
	}
	if loaded.Workspaces[firstName].Path != firstRoot || loaded.Workspaces[secondName].Path != secondRoot {
		t.Fatalf("migrated workspaces = %#v", loaded.Workspaces)
	}
}

func TestLoadRejectsAmbiguousRelativeLegacyWorkspaceDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `version: 1
default_workspace: workspaces/dev
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "absolute") || !strings.Contains(err.Error(), "workspace create") {
		t.Fatalf("Load() error = %v", err)
	}
}

func quoteYAML(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

func TestLoadReadsNonSecretEnvironmentProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `version: 1
default_environment: production
environments:
  production:
    url: https://example.test
    site_content_url: marketing
    api_version: "3.29"
    auth:
      type: pat
      pat_name_env: PROD_PAT_NAME
      pat_secret_env: PROD_PAT_SECRET
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := loaded.ResolveEnvironment("")
	if err != nil {
		t.Fatal(err)
	}
	if environment.Alias != "production" || environment.APIVersion != "3.29" || environment.Auth.PATSecretEnv != "PROD_PAT_SECRET" {
		t.Fatalf("environment = %#v", environment)
	}
}

func TestLoadRejectsPersistedPATValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `version: 1
environments:
  production:
    url: https://example.test
    auth:
      type: pat
      pat_name: forbidden
      pat_secret: forbidden
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() accepted persisted PAT values")
	}
}

func TestLoadRejectsTrailingYAMLDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `version: 1
environments: {}
---
version: 1
default_environment: production
environments:
  production:
    url: https://unexpected.example.test
    auth:
      type: pat
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() accepted multiple YAML documents")
	}
}

func TestSaveAtomicallyPersistsWorkspaceRegistry(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	cfg := config.Config{
		Version:          config.CurrentVersion,
		DefaultWorkspace: "development",
		Workspaces: map[string]config.WorkspaceRegistration{
			"development": {ID: "ws_11111111111111111111111111111111", Path: filepath.Join(directory, "workspace")},
		},
	}

	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workspaces["development"].ID != cfg.Workspaces["development"].ID {
		t.Fatalf("workspace registry = %#v", loaded.Workspaces)
	}
	cfg.DefaultWorkspace = ""
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("replace configuration: %v", err)
	}
	loaded, err = config.Load(path)
	if err != nil || loaded.DefaultWorkspace != "" {
		t.Fatalf("replaced configuration = %#v, %v", loaded, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		t.Fatalf("configuration directory entries = %#v", entries)
	}
}
