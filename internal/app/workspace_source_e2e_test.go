package app_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

func TestWorkspaceStatusFullJSONPreservesArtifactSourceIdentity(t *testing.T) {
	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "workspace")
	options := app.Options{ConfigPath: filepath.Join(root, "config.yaml")}
	run := func(args ...string) []byte {
		t.Helper()
		var stdout bytes.Buffer
		if exit := app.Run(t.Context(), args, &stdout, options); exit != 0 {
			t.Fatalf("Run(%v) exit = %d, output = %s", args, exit, stdout.String())
		}
		return stdout.Bytes()
	}
	run("workspace", "create", "inventory", "--path", workspaceRoot)

	// A LUID and display name can recur across sites and servers. Preserve the
	// acquired source identity without resolving current environment aliases.
	sources := []struct {
		origin, siteLUID, environment, site string
	}{
		{"https://alpha.example.test", "site-a", "source-a", "engineering"},
		{"https://alpha.example.test", "site-b", "source-b", "sandbox"},
		{"https://beta.example.test", "site-a", "source-c", ""},
	}
	manager := artifact.NewWorkbookManager(nil)
	for _, source := range sources {
		_, err := manager.Pull(t.Context(), artifact.WorkbookPull{
			Workspace: workspaceRoot,
			Filename:  "Inventory.twb",
			Content:   []byte("<workbook/>"),
			Metadata: artifact.WorkbookMetadata{
				Name:               "Inventory",
				TableauID:          "workbook-1",
				SourceServerOrigin: source.origin,
				SourceSiteLUID:     source.siteLUID,
				SourceEnvironment:  source.environment,
				SourceSite:         source.site,
				SourceProjectName:  "Examples",
				SourceProjectID:    "project-1",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	decodeArtifacts := func(data []byte) []map[string]any {
		t.Helper()
		var result struct {
			Inventory struct {
				Artifacts []map[string]any `json:"artifacts"`
			} `json:"artifacts"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("decode workspace status: %v\n%s", err, data)
		}
		if len(result.Inventory.Artifacts) != len(sources) {
			t.Fatalf("artifacts = %d, want %d: %s", len(result.Inventory.Artifacts), len(sources), data)
		}
		return result.Inventory.Artifacts
	}
	full := decodeArtifacts(run("workspace", "status", "--workspace", "inventory", "--all", "--full", "--json"))
	byEnvironment := make(map[string]map[string]any, len(full))
	for _, item := range full {
		environment, ok := item["source_environment"].(string)
		if !ok {
			t.Fatalf("artifact omits source_environment: %#v", item)
		}
		if _, duplicate := byEnvironment[environment]; duplicate {
			t.Fatalf("duplicate source environment %q: %#v", environment, full)
		}
		byEnvironment[environment] = item
	}
	for _, source := range sources {
		item := byEnvironment[source.environment]
		for field, want := range map[string]string{
			"source_server_origin": source.origin,
			"source_site_luid":     source.siteLUID,
			"luid":                 "workbook-1",
			"state":                "clean",
		} {
			if item[field] != want {
				t.Errorf("source %s %s = %v, want %q", source.environment, field, item[field], want)
			}
		}
		if source.site != "" && item["source_site"] != source.site {
			t.Errorf("source %s site = %v, want %q", source.environment, item["source_site"], source.site)
		}
		if source.site == "" {
			if value, present := item["source_site"]; present && value != "" {
				t.Errorf("default-site source contains invented site label: %v", value)
			}
		}
	}

	compact := decodeArtifacts(run("workspace", "status", "--workspace", "inventory", "--all", "--json"))
	for _, item := range compact {
		for _, field := range []string{"source_server_origin", "source_site_luid", "source_environment", "source_site"} {
			if value, present := item[field]; present {
				t.Errorf("compact status exposes %s = %v", field, value)
			}
		}
	}
}
