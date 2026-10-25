package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func testProfileConfig(t *testing.T, server, site string, enabled *bool) (Profile, string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.yaml")
	env := config.Environment{URL: server, SiteContentURL: site, Auth: config.Auth{Type: "pat", PATNameEnv: "LIVE_TEST_ACTOR_NAME", PATSecretEnv: "LIVE_TEST_ACTOR_SECRET"}}
	cfg := config.Config{Version: 1, DefaultEnvironment: "test", Environments: map[string]config.Environment{"test": env}}
	if enabled != nil {
		if err := cfg.SetMutationSetting(env, *enabled); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.Save(source, cfg); err != nil {
		t.Fatal(err)
	}
	p := Profile{Version: 1, Alias: "test", ServerURL: server, SiteContentURL: site, ConfigPath: source, Environment: "test", ActorPAT: Credential{NameEnv: "LIVE_TEST_ACTOR_NAME", SecretEnv: "LIVE_TEST_ACTOR_SECRET"}, FixturePAT: Credential{NameEnv: "LIVE_TEST_FIXTURE_NAME", SecretEnv: "LIVE_TEST_FIXTURE_SECRET"}, Budgets: Budgets{5, 5, 5}}
	return p, dir
}

func TestProfileRequiresExplicitSiteIncludingEmptyServerDefault(t *testing.T) {
	base := map[string]any{"schema_version": 1, "alias": "test", "server_url": "https://tableau.example.test", "config_path": "source.yaml", "environment": "test", "actor_pat": map[string]string{"name_env": "PAT_NAME", "secret_env": "PAT_SECRET"}}
	for _, mode := range []string{"omitted", "null", "empty", "named"} {
		t.Run(mode, func(t *testing.T) {
			fields := map[string]any{}
			for key, value := range base {
				fields[key] = value
			}
			switch mode {
			case "null":
				fields["site_content_url"] = nil
			case "empty":
				fields["site_content_url"] = ""
			case "named":
				fields["site_content_url"] = "fixture-site"
			}
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(t.TempDir(), "profile.json")
			if err := os.WriteFile(filename, data, 0600); err != nil {
				t.Fatal(err)
			}
			p, err := LoadProfile(filename)
			valid := mode == "empty" || mode == "named"
			if valid && err != nil {
				t.Fatalf("explicit site rejected: %v", err)
			}
			if !valid && err == nil {
				t.Fatal("missing explicit site accepted")
			}
			if valid && mode == "empty" && p.SiteContentURL != "" {
				t.Fatal("default-site identity changed")
			}
		})
	}
}

func TestConsentUsesCanonicalServerAndExactSiteWithoutWriting(t *testing.T) {
	for _, setting := range []string{"absent", "disabled", "enabled"} {
		t.Run(setting, func(t *testing.T) {
			var consent *bool
			if setting != "absent" {
				consent = new(setting == "enabled")
			}
			p, _ := testProfileConfig(t, "https://TABLEAU.example.test:443/", "", consent)
			before, err := os.ReadFile(p.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			p.ServerURL = "https://tableau.example.test"
			err = p.VerifyConsent()
			if setting == "enabled" && err != nil {
				t.Fatalf("equivalent canonical target rejected: %v", err)
			}
			if setting != "enabled" && ErrorStatus(err) != BlockedStatus {
				t.Fatalf("missing consent was not blocked: %v", err)
			}
			after, err := os.ReadFile(p.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("consent verification modified the saved source configuration")
			}
			p.SiteContentURL = "other-site"
			if err := p.VerifyConsent(); err == nil {
				t.Fatal("another site inherited consent")
			}
		})
	}
}
