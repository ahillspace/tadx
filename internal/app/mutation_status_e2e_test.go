package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/toon"
)

func mutationStatusOptions(t *testing.T) Options {
	t.Helper()
	options := overviewOptions(t, t.TempDir())
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateUnmanaged, allowed: map[string]bool{"mutation.status": true}, remote: true}
	cfg := config.Config{Version: 1, DefaultEnvironment: "dev", Environments: map[string]config.Environment{
		"dev":     {URL: "https://TABLEAU.example.com:443/", SiteContentURL: "shared", Auth: config.Auth{Type: "pat"}},
		"alias":   {URL: "https://tableau.example.com", SiteContentURL: "shared", Auth: config.Auth{Type: "pat"}},
		"other":   {URL: "https://other.example.com", SiteContentURL: "shared", Auth: config.Auth{Type: "pat"}},
		"prod":    {URL: "https://tableau.example.com", SiteContentURL: "production", Auth: config.Auth{Type: "pat"}},
		"default": {URL: "https://tableau.example.com", Auth: config.Auth{Type: "pat"}},
	}, SiteMutations: []config.SiteMutation{
		{ServerURL: "https://tableau.example.com", SiteContentURL: "shared", Enabled: true},
		{ServerURL: "https://tableau.example.com", SiteContentURL: "production", Enabled: false},
	}}
	if err := config.Save(options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	return options
}

func runMutationStatus(t *testing.T, options Options, args ...string) (int, string) {
	t.Helper()
	before, err := os.ReadFile(options.ConfigPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := Run(t.Context(), args, &output, options)
	after, err := os.ReadFile(options.ConfigPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("status changed configuration")
	}
	return code, output.String()
}

func TestMutationStatusAllEnvironmentsThroughCLI(t *testing.T) {
	options := mutationStatusOptions(t)
	t.Setenv("TADX_ENVIRONMENT", "other")
	t.Setenv("TADX_ENABLE_MUTATIONS", "0")
	const want = "sites[5]{environment,enabled}:\n  alias,true\n  default,false\n  dev,true\n  other,false\n  prod,false\n"
	for _, defaultEnvironment := range []string{"dev", ""} {
		if _, err := config.Update(options.ConfigPath, false, func(cfg config.Config) (config.Config, error) {
			cfg.DefaultEnvironment = defaultEnvironment
			return cfg, nil
		}); err != nil {
			t.Fatal(err)
		}
		code, got := runMutationStatus(t, options, "mutation", "status")
		if code != 0 || got != want {
			t.Errorf("default=%q code=%d output=%s; want %s", defaultEnvironment, code, got, want)
		}
	}
}

func TestMutationStatusExplicitEnvironmentThroughCLI(t *testing.T) {
	options := mutationStatusOptions(t)
	for _, args := range [][]string{
		{"mutation", "status", "--environment", "other"},
		{"--environment", "other", "mutation", "status"},
		{"mutation", "--environment", "other", "status"},
		{"mutation", "status", "--env", "other"},
	} {
		code, got := runMutationStatus(t, options, args...)
		if code != 0 || got != "sites[1]{environment,enabled}:\n  other,false\n" {
			t.Errorf("args=%v code=%d output=%s", args, code, got)
		}
	}
	code, got := runMutationStatus(t, options, "mutation", "status", "--environment", "missing", "--json")
	if code == 0 || !json.Valid([]byte(got)) || !strings.Contains(got, "environment.resolve") || strings.Contains(got, `"sites"`) {
		t.Fatalf("unknown target: code=%d output=%s", code, got)
	}
}

func TestMutationStatusEmptyConfigurationThroughCLI(t *testing.T) {
	for _, missing := range []bool{false, true} {
		options := overviewOptions(t, t.TempDir())
		options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateUnmanaged, allowed: map[string]bool{"mutation.status": true}, remote: true}
		if !missing {
			if err := config.Save(options.ConfigPath, config.Config{Version: 1}); err != nil {
				t.Fatal(err)
			}
		}
		code, got := runMutationStatus(t, options, "mutation", "status", "--json")
		if code != 0 || strings.TrimSpace(got) != `{"sites":[]}` {
			t.Errorf("missing=%v code=%d output=%s", missing, code, got)
		}
		code, got = runMutationStatus(t, options, "mutation", "status")
		if code != 0 || got != "sites: []\n" {
			t.Errorf("missing=%v code=%d output=%s", missing, code, got)
		}
	}
}

func TestMutationStatusOutputModesThroughCLI(t *testing.T) {
	options := mutationStatusOptions(t)
	const fullWant = `{"sites":[{"environment":"alias","enabled":true,"server_url":"https://tableau.example.com","site_content_url":"shared","source":"saved_site_setting"},{"environment":"default","enabled":false,"server_url":"https://tableau.example.com","site_content_url":"","source":"default_disabled"},{"environment":"dev","enabled":true,"server_url":"https://tableau.example.com","site_content_url":"shared","source":"saved_site_setting"},{"environment":"other","enabled":false,"server_url":"https://other.example.com","site_content_url":"shared","source":"default_disabled"},{"environment":"prod","enabled":false,"server_url":"https://tableau.example.com","site_content_url":"production","source":"saved_site_setting"}]}`
	for _, full := range []bool{false, true} {
		args := []string{"mutation", "status"}
		if full {
			args = append(args, "--full")
		}
		code, got := runMutationStatus(t, options, args...)
		if code != 0 {
			t.Fatalf("code=%d output=%s", code, got)
		}
		toonValue, err := toon.Decode([]byte(got))
		if err != nil {
			t.Fatal(err)
		}
		code, got = runMutationStatus(t, options, append(args, "--json")...)
		var jsonValue any
		if code != 0 || json.Unmarshal([]byte(got), &jsonValue) != nil {
			t.Fatalf("code=%d output=%s", code, got)
		}
		if !reflect.DeepEqual(toonValue, jsonValue) {
			t.Fatalf("full=%v TOON/JSON differ: TOON=%v JSON=%v", full, toonValue, jsonValue)
		}
		if full {
			var want any
			if err := json.Unmarshal([]byte(fullWant), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(jsonValue, want) {
				t.Fatalf("full output=%s; want %s", got, fullWant)
			}
		}
	}
}

func TestMutationStatusManagedPolicyThroughCLI(t *testing.T) {
	options := mutationStatusOptions(t)
	for _, full := range []bool{false, true} {
		for _, jsonOutput := range []bool{false, true} {
			args := []string{"mutation", "status", "--environment", "dev"}
			if full {
				args = append(args, "--full")
			}
			if jsonOutput {
				args = append(args, "--json")
			}
			for _, remote := range []bool{false, true} {
				options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"mutation.status": true}, remote: remote}
				code, got := runMutationStatus(t, options, args...)
				if code != 0 {
					t.Fatalf("code=%d output=%s", code, got)
				}
				var result struct {
					Sites []struct {
						Enabled bool `json:"enabled"`
					} `json:"sites"`
					Restriction string `json:"restriction"`
				}
				data := []byte(got)
				if !jsonOutput {
					decoded, err := toon.Decode(data)
					if err != nil {
						t.Fatal(err)
					}
					data, err = json.Marshal(decoded)
					if err != nil {
						t.Fatal(err)
					}
				}
				if err := json.Unmarshal(data, &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Sites) != 1 || !result.Sites[0].Enabled || (result.Restriction == "") != remote {
					t.Fatalf("remote=%v consent/restriction lost: %s", remote, got)
				}
				if !remote && (!strings.Contains(result.Restriction, "blocks remote mutations") || !strings.Contains(result.Restriction, "site consent only")) {
					t.Fatalf("restriction is unclear: %s", got)
				}
			}
		}
	}
	for _, policy := range []fixtureManagedPolicy{
		{state: managedpolicy.StateActive, remote: true},
		{state: managedpolicy.StateError, allowed: map[string]bool{"mutation.status": true}},
	} {
		options.managedPolicy = policy
		code, got := runMutationStatus(t, options, "mutation", "status", "--json")
		if code == 0 || !strings.Contains(got, "policy.denied") || strings.Contains(got, `"sites"`) {
			t.Fatalf("policy denial bypassed: code=%d output=%s", code, got)
		}
	}
}

func TestMutationStatusHelpThroughCLI(t *testing.T) {
	options := mutationStatusOptions(t)
	for _, args := range [][]string{{"mutation", "--help"}, {"mutation", "status", "--help"}} {
		code, got := runMutationStatus(t, options, args...)
		if code != 0 || strings.Contains(got, "reads: default") {
			t.Fatalf("incorrect selection guidance: code=%d output=%s", code, got)
		}
		if strings.Count(got, "all configured environments") != 1 {
			t.Errorf("help must explain the inventory scope once: %s", got)
		}
		for _, want := range []string{"--environment", "site consent only", "--full", "server_url", "site_content_url", "source"} {
			if !strings.Contains(got, want) {
				t.Errorf("help omits %q: %s", want, got)
			}
		}
	}
}
