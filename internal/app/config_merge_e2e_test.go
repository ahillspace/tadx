package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestConfigYAMLMergeAuthStatusThroughCLI(t *testing.T) {
	path := resilienceConfig(t, "")
	settings := `version: 1
environments:
  good: &base
    url: https://example.test
    auth: {type: pat}
  other:
    <<: *base
    site_content_url: other
`
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
	out := resilienceRequire(t, options, "auth", "status", "--env", "other", "--json", "--full")
	var status struct {
		Environment string `json:"environment"`
		ServerURL   string `json:"server_url"`
		Site        string `json:"site_content_url"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil || status.Environment != "other" || status.ServerURL != "https://example.test" || status.Site != "other" {
		t.Fatalf("merged local auth status = %s err=%v", out, err)
	}
	resilienceRequire(t, options, "env", "update", "other", "--site", "updated", "--json")
	resilienceRequire(t, options, "auth", "status", "--env", "other", "--json")
}

func TestConfigWorkspaceRepairExactNameThroughCLI(t *testing.T) {
	path := resilienceConfig(t, "")
	settings := `version: 1
workspaces:
  Good: {id: ws_11111111111111111111111111111111, path: first/root}
  good: {id: ws_22222222222222222222222222222222, path: second/root}
`
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	options := Options{ConfigPath: path}
	code, out := resilienceRun(t, options, "workspace", "unregister", "GOOD", "--json")
	if code == 0 || !strings.Contains(out, "ambiguous") {
		t.Fatalf("ambiguous unregister did not fail: %s", out)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != settings {
		t.Fatal("ambiguous unregister writes settings")
	}
	out = resilienceRequire(t, options, "workspace", "unregister", "good", "--json")
	var removed struct {
		Workspace struct {
			Name string `json:"name"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(out), &removed); err != nil || removed.Workspace.Name != "good" {
		t.Fatalf("exact unregister removes wrong entry: %s err=%v", out, err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := c.Workspaces["Good"]; !exists || len(c.Workspaces) != 1 {
		t.Fatal("exact unregister loses the other registration")
	}
}

func TestConfigWorkspaceStatusPreservesExactInvalidNameThroughCLI(t *testing.T) {
	path := resilienceConfig(t, "")
	settings := `version: 1
workspaces:
  Good: {id: ws_11111111111111111111111111111111, path: first/root}
  good: {id: ws_22222222222222222222222222222222, path: second/root}
`
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	options := Options{ConfigPath: path}
	code, out := resilienceRun(t, options, "workspace", "status", "--workspace", "good", "--json", "--full")
	var result struct {
		Error struct {
			Prerequisite struct {
				Resource string `json:"resource"`
			} `json:"prerequisite"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || code == 0 || result.Error.Prerequisite.Resource != "good" {
		t.Fatalf("explicit status selector diagnoses the wrong registration: %s err=%v", out, err)
	}
	code, out = resilienceRun(t, options, "workspace", "status", "--workspace", "GOOD", "--json")
	if code == 0 || !strings.Contains(out, "ambiguous") {
		t.Fatalf("ambiguous status selector did not fail: %s", out)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != settings {
		t.Fatal("workspace status changes invalid registrations")
	}
}
