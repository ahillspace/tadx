package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/config"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

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

func TestWorkspaceCreateUsesSelectedConfigAfterFlagParsing(t *testing.T) {
	for _, flag := range []string{"--config", "--cfg"} {
		t.Run(flag, func(t *testing.T) {
			root := t.TempDir()
			fallback := filepath.Join(root, "fallback.yaml")
			selected := filepath.Join(root, "selected.yaml")
			for _, path := range []string{fallback, selected} {
				if err := config.Save(path, config.Config{Version: config.CurrentVersion}); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			args := []string{"workspace", "create", "selected", "--path", filepath.Join(root, "selected-workspace"), flag, selected}
			if code := Run(t.Context(), args, &output, Options{ConfigPath: fallback}); code != 0 {
				t.Fatalf("code=%d output=%s", code, output.String())
			}
			selectedConfig, err := config.Load(selected)
			if err != nil {
				t.Fatal(err)
			}
			fallbackConfig, err := config.Load(fallback)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := selectedConfig.Workspaces["selected"]; !ok {
				t.Fatalf("selected workspace missing: %#v", selectedConfig.Workspaces)
			}
			if _, ok := fallbackConfig.Workspaces["selected"]; ok {
				t.Fatalf("fallback config was modified: %#v", fallbackConfig.Workspaces)
			}
		})
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
