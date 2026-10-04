package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/managedpolicy"
)

func selectedConfigFixture(t *testing.T, options Options, selected config.Config, fallback config.Config) string {
	t.Helper()
	selectedPath := filepath.Join(t.TempDir(), "selected.yaml")
	if err := config.Save(selectedPath, selected); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(options.ConfigPath, fallback); err != nil {
		t.Fatal(err)
	}
	return selectedPath
}

func runSelectedConfig(t *testing.T, options Options, selectedPath string, args ...string) (int, string) {
	t.Helper()
	var output bytes.Buffer
	code := Run(t.Context(), append([]string{"--config", selectedPath}, args...), &output, options)
	return code, output.String()
}

func TestSelectedConfigCacheOperationsBindAfterFlagParsing(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	selected := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://selected.example.test", SiteContentURL: "selected-site", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	fallback := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"fallback": {URL: "https://fallback.example.test", SiteContentURL: "fallback-site", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	path := selectedConfigFixture(t, options, selected, fallback)
	for _, args := range [][]string{
		{"cache", "refresh", "--environment", "dev", "--preview", "--json"},
		{"cache", "status", "--environment", "dev", "--json"},
	} {
		code, out := runSelectedConfig(t, options, path, args...)
		if code != 0 || !strings.Contains(out, `"site":"selected-site"`) || strings.Contains(out, "fallback-site") {
			t.Fatalf("selected cache %v: code=%d output=%s", args, code, out)
		}
	}
	if code, out := runSelectedConfig(t, options, path, "cache", "status", "--environment", "fallback", "--json"); code == 0 || !strings.Contains(out, `"id":"cache.status.setup"`) || !strings.Contains(out, "Available aliases: dev.") {
		t.Fatalf("fallback-only alias accepted: code=%d output=%s", code, out)
	}
}

func TestSelectedConfigEnvironmentReadsAndWritesOnlySelectedFile(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	selected := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://selected.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	fallback := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://fallback.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	path := selectedConfigFixture(t, options, selected, fallback)
	if code, out := runSelectedConfig(t, options, path, "env", "get", "dev", "--json"); code != 0 || !strings.Contains(out, "https://selected.example.test") || strings.Contains(out, "https://fallback.example.test") {
		t.Fatalf("selected read code=%d output=%s", code, out)
	}
	if code, out := runSelectedConfig(t, options, path, "env", "update", "dev", "--site", "selected-site"); code != 0 {
		t.Fatalf("selected update code=%d output=%s", code, out)
	}
	gotSelected, err := config.Load(path)
	if err != nil || gotSelected.Environments["dev"].SiteContentURL != "selected-site" {
		t.Fatalf("selected=%#v err=%v", gotSelected.Environments["dev"], err)
	}
	gotFallback, err := config.Load(options.ConfigPath)
	if err != nil || gotFallback.Environments["dev"].SiteContentURL != "" {
		t.Fatalf("fallback=%#v err=%v", gotFallback.Environments["dev"], err)
	}
}

func TestSelectedConfigCredentialGuardCannotUseFallbackProfile(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	selected := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://selected.example.test", Auth: config.Auth{Type: config.AuthTypePAT, CredentialRef: "cred_0123456789abcdef0123456789abcdef"}},
	}}
	fallback := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://fallback.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	path := selectedConfigFixture(t, options, selected, fallback)
	if code, out := runSelectedConfig(t, options, path, "env", "update", "dev", "--url", "https://retarget.example.test"); code == 0 || !strings.Contains(out, "auth logout") {
		t.Fatalf("credential guard code=%d output=%s", code, out)
	}
	gotSelected, err := config.Load(path)
	if err != nil || gotSelected.Environments["dev"].URL != "https://selected.example.test" {
		t.Fatalf("selected=%#v err=%v", gotSelected.Environments["dev"], err)
	}
	gotFallback, err := config.Load(options.ConfigPath)
	if err != nil || gotFallback.Environments["dev"].URL != "https://fallback.example.test" {
		t.Fatalf("fallback=%#v err=%v", gotFallback.Environments["dev"], err)
	}
}

func TestSelectedConfigMutationConsentUsesExactSelectedSite(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	selected := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://SELECTED.example.test:443/", SiteContentURL: "qa", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	fallback := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://fallback.example.test", SiteContentURL: "qa", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	path := selectedConfigFixture(t, options, selected, fallback)
	if code, out := runSelectedConfig(t, options, path, "mutation", "set", "--environment", "dev", "--enabled=true"); code != 0 || !strings.Contains(out, "persisted: true") {
		t.Fatalf("consent write code=%d output=%s", code, out)
	}
	gotSelected, err := config.Load(path)
	if err != nil || len(gotSelected.SiteMutations) != 1 || gotSelected.SiteMutations[0].ServerURL != "https://selected.example.test" || gotSelected.SiteMutations[0].SiteContentURL != "qa" || !gotSelected.SiteMutations[0].Enabled {
		t.Fatalf("selected consent=%#v err=%v", gotSelected.SiteMutations, err)
	}
	gotFallback, err := config.Load(options.ConfigPath)
	if err != nil || len(gotFallback.SiteMutations) != 0 {
		t.Fatalf("fallback consent=%#v err=%v", gotFallback.SiteMutations, err)
	}
	if code, out := runSelectedConfig(t, options, path, "mutation", "status", "--environment", "dev", "--json"); code != 0 || !strings.Contains(out, `"enabled":true`) {
		t.Fatalf("selected status code=%d output=%s", code, out)
	}
}

func TestSelectedConfigMutationSetCannotUseFallbackAlias(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	selected := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"other": {URL: "https://selected.example.test", SiteContentURL: "qa", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	fallback := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://fallback.example.test", SiteContentURL: "qa", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	path := selectedConfigFixture(t, options, selected, fallback)
	if code, out := runSelectedConfig(t, options, path, "mutation", "set", "--environment", "dev", "--enabled=true"); code == 0 || !strings.Contains(out, "Select one configured environment") {
		t.Fatalf("selected authority code=%d output=%s", code, out)
	}
	gotSelected, err := config.Load(path)
	if err != nil || len(gotSelected.SiteMutations) != 0 {
		t.Fatalf("selected consent=%#v err=%v", gotSelected.SiteMutations, err)
	}
	gotFallback, err := config.Load(options.ConfigPath)
	if err != nil || len(gotFallback.SiteMutations) != 0 {
		t.Fatalf("fallback consent=%#v err=%v", gotFallback.SiteMutations, err)
	}
}

func TestSelectedConfigCapabilityReadinessCannotUseFallbackConsent(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, remote: true, allowed: map[string]bool{"capability.list": true, "project.create": true}}
	selected := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://tableau.example.test", SiteContentURL: "qa", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	fallback := selected
	fallback.SiteMutations = []config.SiteMutation{{ServerURL: "https://tableau.example.test", SiteContentURL: "qa", Enabled: true}}
	path := selectedConfigFixture(t, options, selected, fallback)
	code, out := runSelectedConfig(t, options, path, "capability", "list", "--environment", "dev", "--domain", "content", "--resource", "project", "--mutation=true", "--json")
	if code != 0 {
		t.Fatalf("capability list code=%d output=%s", code, out)
	}
	var result struct {
		Capabilities []struct {
			ID               string `json:"id"`
			ExecutionEnabled bool   `json:"execution_enabled"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range result.Capabilities {
		if item.ID == "project.create" {
			found = true
			if item.ExecutionEnabled {
				t.Fatalf("fallback consent enabled selected-site capability: %s", out)
			}
		}
	}
	if !found {
		t.Fatalf("project.create missing from capability output: %s", out)
	}
}

func TestSelectedConfigCapabilityReadinessUsesSelectedConsent(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, remote: true, allowed: map[string]bool{"capability.list": true, "project.create": true}}
	selected := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: "https://tableau.example.test", SiteContentURL: "qa", Auth: config.Auth{Type: config.AuthTypePAT}},
	}, SiteMutations: []config.SiteMutation{{ServerURL: "https://tableau.example.test", SiteContentURL: "qa", Enabled: true}}}
	fallback := selected
	fallback.SiteMutations = nil
	path := selectedConfigFixture(t, options, selected, fallback)
	code, out := runSelectedConfig(t, options, path, "capability", "list", "--environment", "dev", "--domain", "content", "--resource", "project", "--mutation=true", "--json")
	if code != 0 {
		t.Fatalf("capability list code=%d output=%s", code, out)
	}
	var result struct {
		Capabilities []struct {
			ID               string `json:"id"`
			ExecutionEnabled bool   `json:"execution_enabled"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Capabilities {
		if item.ID == "project.create" {
			if !item.ExecutionEnabled {
				t.Fatalf("selected consent did not enable selected-site capability: %s", out)
			}
			return
		}
	}
	t.Fatalf("project.create missing from capability output: %s", out)
}
