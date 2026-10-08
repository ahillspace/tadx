package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/workspace"
	"gopkg.in/yaml.v3"
)

func workspaceResilienceFixture(t *testing.T, nested bool) (*workspace.Manager, string, workspace.Record, string) {
	t.Helper()
	base := t.TempDir()
	path := filepath.Join(base, "config.yaml")
	manager := workspace.NewManager(path, nil)
	record, err := manager.Create(t.Context(), "good", filepath.Join(base, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(base, "invalid")
	if nested {
		child = filepath.Join(record.Root, "child")
	}
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "keep.txt"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["workspaces"].(map[string]any)["broken"] = map[string]any{"id": "invalid-identity", "path": filepath.ToSlash(child)}
	data, err = yaml.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return manager, path, record, child
}

func TestWorkspaceResiliencePreservesTypedInvalidSelection(t *testing.T) {
	manager, _, _, _ := workspaceResilienceFixture(t, false)
	if _, err := manager.Resolve(t.Context(), "good", ""); err != nil {
		t.Fatalf("unrelated invalid registration blocked healthy workspace: %v", err)
	}
	_, err := manager.Resolve(t.Context(), "broken", "")
	invalid, ok := errors.AsType[*config.InvalidWorkspaceError](err)
	if !ok || len(invalid.CorrectiveCommands()) == 0 || !slices.Equal(invalid.CorrectiveCommands()[0], []string{"workspace", "unregister", "broken"}) {
		t.Fatalf("manager discarded typed invalid workspace recovery: %v", err)
	}
}

func TestWorkspaceResilienceUnregisterKeepsFilesAndCountsAllEntries(t *testing.T) {
	manager, path, _, child := workspaceResilienceFixture(t, false)
	page, err := manager.List(t.Context(), 20, 0)
	if err != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("registry listing lost invalid entry: page=%+v err=%v", page, err)
	}
	for _, record := range page.Items {
		if record.Name == "broken" && (record.ID != "" || record.Root != "") {
			t.Fatal("invalid registration rendered entry values")
		}
	}
	if _, err := manager.PreviewUnregister(t.Context(), "broken"); err != nil {
		t.Fatalf("invalid registration could not be previewed for removal: %v", err)
	}
	if _, err := manager.Unregister(t.Context(), "broken"); err != nil {
		t.Fatalf("invalid registration could not be removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(child, "keep.txt")); err != nil {
		t.Fatal("unregister changed workspace files")
	}
	cfg, err := config.Load(path)
	if err != nil || len(cfg.WorkspaceNames()) != 1 {
		t.Fatalf("invalid registration remained saved: err=%v", err)
	}
}

func TestWorkspaceResilienceInvalidChildProtectsParentDeletion(t *testing.T) {
	manager, _, parent, child := workspaceResilienceFixture(t, true)
	if _, err := manager.Delete(t.Context(), parent); err == nil || !strings.Contains(err.Error(), "contains registered workspace") {
		t.Fatalf("parent deletion ignored invalid child registration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(child, "keep.txt")); err != nil {
		t.Fatal("parent deletion removed invalid child files")
	}
}

func TestWorkspaceResilienceKnownOutsidePathDoesNotBlockDeletion(t *testing.T) {
	manager, path, parent, outside := workspaceResilienceFixture(t, false)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultWorkspace = ""
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Delete(t.Context(), parent); err != nil {
		t.Fatalf("invalid outside registration blocked safe deletion: %v", err)
	}
	if _, err := os.Stat(parent.Root); !os.IsNotExist(err) {
		t.Fatal("safe deletion retained its selected root", err)
	}
	if contents, err := os.ReadFile(filepath.Join(outside, "keep.txt")); err != nil || string(contents) != "preserve" {
		t.Fatal("safe deletion changed outside registration files", err)
	}
	cfg, err = config.Load(path)
	if err != nil || len(cfg.WorkspaceNames()) != 1 || len(cfg.InvalidWorkspaces) != 1 || cfg.WorkspaceRegistrations()["broken"].Path != filepath.ToSlash(outside) {
		t.Fatalf("safe deletion discarded the invalid outside registration: config=%+v err=%v", cfg, err)
	}
}

func TestWorkspaceResilienceRegistrationRejectsInvalidOccupiedRoot(t *testing.T) {
	manager, _, _, child := workspaceResilienceFixture(t, false)
	if _, err := manager.PreviewCreate(t.Context(), "broken", filepath.Join(t.TempDir(), "new")); err == nil {
		t.Fatal("create reused an invalid entry's registered name")
	}
	if _, err := manager.PreviewCreate(t.Context(), "another", child); err == nil {
		t.Fatal("create reused an invalid entry's occupied root")
	}
}

func TestWorkspaceResilienceInvalidDefaultRequiresReplacement(t *testing.T) {
	manager, path, _, _ := workspaceResilienceFixture(t, false)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "default_workspace: good", "default_workspace: broken"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Unregister(t.Context(), "broken"); err == nil {
		t.Fatal("invalid default registration bypassed replacement guard")
	}
	if _, err := manager.SetDefault(t.Context(), "good"); err != nil {
		t.Fatalf("invalid default prevented choosing a healthy replacement: %v", err)
	}
	if _, err := manager.Unregister(t.Context(), "broken"); err != nil {
		t.Fatalf("invalid registration remained locked after default replacement: %v", err)
	}
}

func TestWorkspaceResilienceCollisionParticipantsRemainProtected(t *testing.T) {
	manager, path, parent, child := workspaceResilienceFixture(t, true)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	registrations := raw["workspaces"].(map[string]any)
	registrations["broken"].(map[string]any)["id"] = "ws_88888888888888888888888888888888"
	registrations["peer"] = map[string]any{"id": "ws_88888888888888888888888888888888", "path": filepath.ToSlash(filepath.Join(t.TempDir(), "peer"))}
	data, err = yaml.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"broken", "peer"} {
		_, err := manager.Resolve(t.Context(), name, "")
		invalid, ok := errors.AsType[*config.InvalidWorkspaceError](err)
		if !ok || !strings.Contains(invalid.Error(), "broken") || !strings.Contains(invalid.Error(), "peer") {
			t.Fatalf("collision participant lacked shared violation: %v", err)
		}
	}
	if _, err := manager.Delete(t.Context(), parent); err == nil {
		t.Fatal("colliding invalid child lost filesystem deletion protection")
	}
	if _, err := os.Stat(filepath.Join(child, "keep.txt")); err != nil {
		t.Fatal("collision participant files were removed")
	}
}

func TestWorkspaceResilienceResolvesEnvironmentOnlyWhenItsDefaultIsNeeded(t *testing.T) {
	manager, path, parent, _ := workspaceResilienceFixture(t, false)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["default_environment"] = "broken-env"
	raw["environments"] = map[string]any{
		"good-env":   map[string]any{"url": "https://tableau.example.test", "auth": map[string]any{"type": "pat"}, "default_workspace": "good"},
		"broken-env": map[string]any{"url": "https://tableau.example.test", "auth": map[string]any{"type": "pat", "pat_secret_env": "fixturePAT==:invalid"}},
	}
	data, err = yaml.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"", "broken-env"} {
		record, err := manager.Resolve(t.Context(), "good", alias)
		if err != nil || record.SelectionReason != "explicit" {
			t.Fatalf("explicit workspace unnecessarily resolved environment: record=%+v err=%v", record, err)
		}
	}
	t.Chdir(parent.Root)
	record, err := manager.Resolve(t.Context(), "", "broken-env")
	if err != nil || record.SelectionReason != "containing_directory" {
		t.Fatalf("containing directory unnecessarily resolved environment: record=%+v err=%v", record, err)
	}
	t.Chdir(t.TempDir())
	for _, alias := range []string{"", "broken-env"} {
		_, err := manager.Resolve(t.Context(), "", alias)
		if _, invalid := errors.AsType[*config.InvalidEnvironmentError](err); !invalid {
			t.Fatalf("needed invalid environment default did not fail: %v", err)
		}
	}
	record, err = manager.Resolve(t.Context(), "", "good-env")
	if err != nil || record.SelectionReason != "environment_default" || record.Name != "good" {
		t.Fatalf("selected environment default did not resolve: record=%+v err=%v", record, err)
	}
}
