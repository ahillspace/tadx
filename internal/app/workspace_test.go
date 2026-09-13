package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceDeletionInspectionTreatsUnmanagedFilesAsDirty(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"artifacts", ".tadx"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty, err := workspaceHasUnmanagedEntries(context.Background(), root, nil)
	if err != nil || dirty {
		t.Fatalf("clean workspace: dirty=%t err=%v", dirty, err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty, err = workspaceHasUnmanagedEntries(context.Background(), root, nil)
	if err != nil || !dirty {
		t.Fatalf("unmanaged file: dirty=%t err=%v", dirty, err)
	}
}

func TestWorkspaceArtifactAdaptersPreserveCleanupWarnings(t *testing.T) {
	item := artifact.Item{Warnings: []string{"cleanup remains"}}
	if got := moveArtifact(item).Warnings; !reflect.DeepEqual(got, item.Warnings) {
		t.Fatalf("move warnings = %#v", got)
	}
	if got := deleteArtifact(item).Warnings; !reflect.DeepEqual(got, item.Warnings) {
		t.Fatalf("delete warnings = %#v", got)
	}
}

func TestMovedMetadataCapabilitiesAreClassifiedUnderCatalog(t *testing.T) {
	for id, wantResource := range map[string]string{"lineage.pull": "lineage", "content.label.list": "label", "content.label.inspect": "label", "content.label.update": "label", "content.label.delete": "label"} {
		definition, ok := capability.Lookup(id)
		if !ok {
			t.Fatalf("%s is missing from the registry", id)
		}
		domain, resource := classify(definition)
		if domain != "catalog" || resource != wantResource {
			t.Fatalf("classify(%s) = %q, %q", id, domain, resource)
		}
	}
}

func TestNestedCapabilityIDUsesItsFirstResourceSegment(t *testing.T) {
	definition, ok := capability.Lookup("admin.group.member.add")
	if !ok {
		t.Fatal("admin.group.member.add is missing from the registry")
	}
	domain, resource := classify(definition)
	if domain != "admin" || resource != "group" {
		t.Fatalf("classify(admin.group.member.add) = %q, %q", domain, resource)
	}
}

func TestWorkspaceResolutionUsesSelectedEnvironmentDefault(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	manager := workspacecore.NewManager(configPath, strings.NewReader(strings.Repeat("a", 16)+strings.Repeat("b", 16)))
	if _, err := manager.Create(context.Background(), "general", filepath.Join(root, "general")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(context.Background(), "production", filepath.Join(root, "production")); err != nil {
		t.Fatal(err)
	}
	configuration, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	configuration.Environments = map[string]config.Environment{
		"production": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT}, DefaultWorkspace: "production"},
	}
	if err := config.Save(configPath, configuration); err != nil {
		t.Fatal(err)
	}
	runtime := &workspaceRuntime{runtime: &runtimeDependencies{configPath: configPath}}
	resolved, err := runtime.resolveForEnvironment(context.Background(), "", "production")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "production" {
		t.Fatalf("resolved workspace = %#v", resolved)
	}
}

func TestWorkspaceStatusSetupFailureRetainsCapabilityContext(t *testing.T) {
	var output bytes.Buffer
	exit := Run(context.Background(), []string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "workspace", "status"}, &output, Options{})
	if exit == 0 || !strings.Contains(output.String(), "operation: workspace.status") || !strings.Contains(output.String(), "corrective_action:") {
		t.Fatalf("exit = %d, output = %s", exit, output.String())
	}
}
