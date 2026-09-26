package app

import (
	"bytes"
	"context"
	"encoding/json"
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
	item := artifact.Item{TreeFingerprint: "private-tree-fingerprint", Warnings: []string{"cleanup remains", "second warning"}}
	moved, deleted := moveArtifact(item), deleteArtifact(item)
	if got := moved.Warnings; !reflect.DeepEqual(got, item.Warnings) {
		t.Fatalf("move warnings = %#v", got)
	}
	if got := deleted.Warnings; !reflect.DeepEqual(got, item.Warnings) {
		t.Fatalf("delete warnings = %#v", got)
	}
	moved.Warnings[0], deleted.Warnings[1] = "changed move", "changed delete"
	if !reflect.DeepEqual(item.Warnings, []string{"cleanup remains", "second warning"}) {
		t.Fatalf("projections modified the source warnings: %v", item.Warnings)
	}
	if got := statusArtifact(item).Diagnostic; got != "cleanup remains" {
		t.Fatalf("status diagnostic = %q", got)
	}
	encoded, err := json.Marshal(deleted)
	if err != nil || bytes.Contains(encoded, []byte("private-tree-fingerprint")) || deleted.TreeFingerprint != item.TreeFingerprint {
		t.Fatalf("delete fingerprint projection: JSON=%s target=%#v error=%v", encoded, deleted, err)
	}
}

func TestWorkspaceRegistrationProjectionRequiresAvailableValidManifest(t *testing.T) {
	for _, available := range []bool{false, true} {
		for _, valid := range []bool{false, true} {
			registration := workspaceRegistration(workspacecore.Record{Name: "example", ID: "ws_1", Root: "root", Available: available, ManifestValid: valid})
			if registration.Registered != (available && valid) {
				t.Fatalf("available=%t valid=%t registration=%#v", available, valid, registration)
			}
			encoded, err := json.Marshal(registration)
			if err != nil || bytes.Contains(encoded, []byte("created_entries")) {
				t.Fatalf("registration JSON=%s error=%v", encoded, err)
			}
		}
	}
}

func TestMovedMetadataCapabilitiesAreClassifiedUnderCatalog(t *testing.T) {
	for id, wantResource := range map[string]string{"lineage.pull": "lineage", "content.label.list": "label", "content.label.inspect": "label", "content.label.update": "label", "content.label.delete": "label"} {
		definition, ok := capability.Lookup(id)
		if !ok {
			t.Fatalf("%s is missing from the registry", id)
		}
		discovery := capability.FromDefinition(definition)
		domain, resource := discovery.Domain, discovery.Resource
		if domain != "catalog" || resource != wantResource {
			t.Fatalf("classify(%s) = %q, %q", id, domain, resource)
		}
	}
}

func TestNestedCapabilityIDUsesItsExecutableResource(t *testing.T) {
	definition, ok := capability.Lookup("admin.group.member.add")
	if !ok {
		t.Fatal("admin.group.member.add is missing from the registry")
	}
	discovery := capability.FromDefinition(definition)
	domain, resource := discovery.Domain, discovery.Resource
	if domain != "admin" || resource != "group-member" {
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

func TestWorkspaceEmptyPositionalsRetainActionUsageErrors(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"create", ""}, "name is required"},
		{[]string{"clone", "", "--name", "copy"}, "source and name are required"},
		{[]string{"set-default", ""}, "workspace name is required"},
		{[]string{"unregister", ""}, "workspace name is required"},
		{[]string{"delete", ""}, "workspace name is required"},
	} {
		opts := overviewOptions(t, t.TempDir())
		code, output := runPreviewCommand(t, append([]string{"workspace"}, test.args...), opts)
		if code == 0 || !strings.Contains(output, test.want) {
			t.Fatalf("%v: code=%d output=%s", test.args, code, output)
		}
		if _, err := os.Stat(opts.ConfigPath); !os.IsNotExist(err) {
			t.Fatalf("invalid input created configuration: %v", err)
		}
	}
}
