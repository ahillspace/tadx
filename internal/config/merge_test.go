package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestConfigYAMLMergePreservesSupportedPrecedence(t *testing.T) {
	settings := `<<: {version: 1}
environments:
  first: &first
    url: https://first.example.test
    auth: &auth {type: pat}
  second: &second
    url: https://second.example.test
    site_content_url: inherited
    auth: {type: pat}
  merged:
    <<: [*first, *second]
    site_content_url: explicit
    auth:
      <<: *auth
      pat_name_env: MERGED_NAME
      pat_secret_env: MERGED_SECRET
`
	path := resilienceFile(t, settings)
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := c.ResolveEnvironment("merged")
	if err != nil || merged.URL != "https://first.example.test" || merged.SiteContentURL != "explicit" || merged.Auth.PATNameEnv != "MERGED_NAME" || merged.Auth.PATSecretEnv != "MERGED_SECRET" {
		t.Fatalf("merged environment lost YAML precedence: %#v err=%v", merged, err)
	}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip, err := reloaded.ResolveEnvironment("merged"); err != nil || roundTrip != merged {
		t.Fatalf("write changed merged environment: %#v err=%v", roundTrip, err)
	}
}

func TestConfigYAMLMergePreservesInheritedQuarantine(t *testing.T) {
	settings := `version: 1
future_template: &template
  url: https://example.test
  auth: {type: pat}
  future_field: opaque
environments:
  good: {url: https://good.example.test, auth: {type: pat}}
  broken: {<<: *template}
`
	path := resilienceFile(t, settings)
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveEnvironment("good"); err != nil {
		t.Fatal(err)
	}
	invalid, exists := c.InvalidEnvironments["broken"]
	if !exists || len(invalid.Violations) != 1 || invalid.Violations[0].Field != "future_field" {
		t.Fatalf("inherited unknown field loses its entry diagnosis: %#v", invalid.Violations)
	}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	reloaded, err := config.Load(path)
	if err != nil || len(reloaded.InvalidEnvironments["broken"].Violations) != 1 {
		t.Fatalf("write loses inherited quarantine: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "future_field: opaque") {
		t.Fatal("write loses inherited invalid content")
	}
}

func TestConfigYAMLMergeDoesNotHideMalformedOrDuplicateFields(t *testing.T) {
	for _, merge := range []string{
		"<<: invalid",
		"<<: [{url: https://one.example.test}, invalid]",
		"<<: {url: https://one.example.test, url: https://two.example.test}",
		"<<: {url: https://one.example.test}\n    <<: {url: https://two.example.test}",
	} {
		t.Run(merge, func(t *testing.T) {
			settings := "version: 1\nenvironments:\n  good: {url: https://good.example.test, auth: {type: pat}}\n  broken:\n    " + merge + "\n    auth: {type: pat}\n"
			path := resilienceFile(t, settings)
			c, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.ResolveEnvironment("good"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.ResolveEnvironment("broken"); err == nil {
				t.Fatal("malformed merge becomes valid")
			}
			if err := config.Save(path, c); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, consent := range []string{
		"<<: {version: 1, site_mutations: [{server_url: https://example.test, enabled: true, future_permission: true}]}",
		"version: 1\nsite_mutations: [{<<: {server_url: https://example.test, enabled: invalid}}]",
		"version: 1\nsite_mutations: [{<<: {server_url: https://example.test, enabled: true}, enabled: true, enabled: false}]",
	} {
		if _, err := config.Load(resilienceFile(t, consent)); err == nil {
			t.Fatal("merged malformed mutation control loads")
		}
	}
}

func TestConfigYAMLMergePreservesValidMutationConsent(t *testing.T) {
	settings := `version: 1
site_mutations:
  - <<: {server_url: https://example.test, site_content_url: inherited, enabled: false}
    site_content_url: exact
    enabled: true
`
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	consent, err := c.MutationSetting(config.Environment{URL: "https://example.test", SiteContentURL: "exact"})
	if err != nil || consent == nil || !*consent {
		t.Fatalf("explicit merged consent is lost: %v err=%v", consent, err)
	}
	if other, err := c.MutationSetting(config.Environment{URL: "https://example.test", SiteContentURL: "inherited"}); err != nil || other != nil {
		t.Fatal("overridden merge target grants consent")
	}
}

func TestConfigYAMLMergePreservesAliasExpansionBounds(t *testing.T) {
	for name, content := range map[string]string{
		"cycle": "future: &self {<<: *self}",
		"depth": "future: " + strings.Repeat("{nested: ", 66) + "value" + strings.Repeat("}", 66),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := config.Load(resilienceFile(t, "version: 1\n"+content+"\n")); err == nil || !strings.Contains(err.Error(), "safe bound") {
				t.Fatalf("unbounded YAML loads: %v", err)
			}
		})
	}
	var settings strings.Builder
	settings.WriteString("version: 1\nfirst: &level0 [value, value]\n")
	for level := 1; level < 17; level++ {
		fmt.Fprintf(&settings, "level%d: &level%d [*level%d, *level%d]\n", level, level, level-1, level-1)
	}
	if _, err := config.Load(resilienceFile(t, settings.String())); err == nil || !strings.Contains(err.Error(), "safe bound") {
		t.Fatalf("unbounded alias expansion loads: %v", err)
	}
}

func TestWorkspaceRepairPrefersExactNameAndRejectsAmbiguity(t *testing.T) {
	settings := `version: 1
workspaces:
  Good: {id: ws_11111111111111111111111111111111, path: first/root}
  good: {id: ws_22222222222222222222222222222222, path: second/root}
`
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	if name, _, err := c.WorkspaceForRepair("good"); err != nil || name != "good" {
		t.Fatalf("exact repair selector selected %q: %v", name, err)
	}
	if _, _, err := c.WorkspaceForRepair("GOOD"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("nonexact ambiguous selector did not fail: %v", err)
	}
	if err := c.RemoveWorkspace("good"); err != nil {
		t.Fatal(err)
	}
	if _, exists := c.InvalidWorkspaces["Good"]; !exists {
		t.Fatal("exact repair removes the wrong case variant")
	}
	if err := config.Save(filepath.Join(t.TempDir(), "settings.yaml"), c); err != nil {
		t.Fatal(err)
	}
	if name, _, err := c.WorkspaceForRepair("GOOD"); err != nil || name != "Good" {
		t.Fatalf("unique case-insensitive repair fails: %q %v", name, err)
	}
}

func TestWorkspaceResolvePreservesExactInvalidNameAndRejectsAmbiguity(t *testing.T) {
	c, err := config.Load(resilienceFile(t, `version: 1
default_workspace: good
workspaces:
  Good: {id: ws_11111111111111111111111111111111, path: first/root}
  good: {id: ws_22222222222222222222222222222222, path: second/root}
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{"good", ""} {
		_, _, err := c.ResolveWorkspace(selector)
		invalid, ok := errors.AsType[*config.InvalidWorkspaceError](err)
		if !ok || invalid.Name != "good" {
			t.Fatalf("selector %q diagnoses the wrong registration: %v", selector, err)
		}
	}
	if _, _, err := c.ResolveWorkspace("GOOD"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("nonexact ambiguous selector did not fail: %v", err)
	}
	if err := c.RemoveWorkspace("Good"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	c, err = config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if name, _, err := c.ResolveWorkspace("GOOD"); err != nil || name != "good" {
		t.Fatalf("unique case-insensitive selector fails: %q %v", name, err)
	}
}

func TestInvalidWorkspaceRecoveryUsesValidRegistrationName(t *testing.T) {
	for _, test := range []struct {
		name         string
		violations   []config.EntryViolation
		registration string
	}{
		{name: "", registration: "<name>"},
		{name: " bad ", registration: "<name>"},
		{name: "CON", registration: "<name>"},
		{name: "Good", violations: []config.EntryViolation{{Field: "name", Rule: "must be unique under case-insensitive matching", OtherEntry: "good"}}, registration: "<name>"},
		{name: "Good", violations: []config.EntryViolation{{Field: "path", Rule: "must not be empty"}}, registration: "Good"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := &config.InvalidWorkspaceError{Name: test.name, Violations: test.violations}
			commands := err.CorrectiveCommands()
			if len(commands) != 2 || commands[0][len(commands[0])-1] != test.name || commands[1][len(commands[1])-1] != test.registration {
				t.Fatalf("recovery loses exact removal or reuses an invalid registration name: %#v", commands)
			}
			if test.registration == "<name>" && !strings.Contains(err.CorrectiveExplanation(), "Replace <name>") {
				t.Fatal("registration name placeholder has no replacement guidance")
			}
		})
	}
}
