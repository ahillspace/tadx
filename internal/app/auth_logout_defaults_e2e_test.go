package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	authops "github.com/ahillspace/tadx/actions/auth"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

func TestLogoutDefaultPATVariablesThroughCLI(t *testing.T) {
	const alias = "logout-review"
	const explicitName = "TADX_LOGOUT_REVIEW_EXPLICIT_NAME"
	const explicitSecret = "TADX_LOGOUT_REVIEW_EXPLICIT_SECRET"
	const invalidVariable = "invalid variable reference"
	defaultName, defaultSecret := config.DefaultPATVariableNames(alias)
	for _, tc := range []struct {
		name, nameVariable, secretVariable string
		secretUnavailable                  bool
		available                          bool
	}{
		{name: "both omitted", available: true},
		{name: "explicit name and default secret", nameVariable: explicitName, available: true},
		{name: "default name and explicit secret", secretVariable: explicitSecret, available: true},
		{name: "both explicit", nameVariable: explicitName, secretVariable: explicitSecret, available: true},
		{name: "unavailable default secret", secretUnavailable: true},
		{name: "invalid name stays invalid", nameVariable: invalidVariable},
		{name: "invalid secret stays invalid", secretVariable: invalidVariable},
		{name: "whitespace name stays invalid", nameVariable: " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(defaultName, "synthetic-review-name")
			t.Setenv(defaultSecret, "synthetic-review-secret")
			t.Setenv(explicitName, "synthetic-review-name")
			t.Setenv(explicitSecret, "synthetic-review-secret")
			if tc.secretUnavailable {
				t.Setenv(defaultSecret, " \t")
			}
			const reference coreauth.CredentialReference = "cred_99999999999999999999999999999999"
			path := filepath.Join(t.TempDir(), "selected.yaml")
			configuration := "version: 1\ndefault_environment: logout-review\nenvironments:\n  logout-review:\n    url: https://tableau.example.test\n    site_content_url: fixture-site\n    auth:\n      type: pat\n"
			if tc.nameVariable != "" {
				configuration += fmt.Sprintf("      pat_name_env: %q\n", tc.nameVariable)
			}
			if tc.secretVariable != "" {
				configuration += fmt.Sprintf("      pat_secret_env: %q\n", tc.secretVariable)
			}
			configuration += "      credential_ref: " + string(reference) + "\n"
			if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {}}}
			network := &warningTestNoNetwork{}
			options := Options{ConfigPath: filepath.Join(t.TempDir(), "unused.yaml"), PATStore: store, HTTPClient: &http.Client{Transport: network}}
			target, err := (authLogoutResolver{runtime: &runtimeDependencies{configPath: path}}).Resolve(t.Context(), alias, true)
			if err != nil {
				t.Fatal("repair-oriented logout resolution failed")
			}
			if target.EnvironmentCredentialsAvailable != tc.available {
				t.Errorf("resolver credential availability = %v, want %v", target.EnvironmentCredentialsAvailable, tc.available)
			}
			if !target.StoredCredentialReferencePresent || target.StoredCredentialReferenceInvalid {
				t.Error("resolver lost the valid stored credential reference")
			}
			for _, preview := range []bool{true, false} {
				args := []string{"--config", path, "auth", "logout", "--environment", alias, "--json"}
				if preview {
					args = append(args, "--preview")
				}
				out := resilienceRequire(t, options, args...)
				for _, variable := range []string{defaultName, defaultSecret, explicitName, explicitSecret} {
					if value := strings.TrimSpace(os.Getenv(variable)); value != "" && strings.Contains(out, value) {
						t.Fatal("logout exposed a synthetic credential value")
					}
				}
				var result authops.LogoutOutput
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal("logout returned invalid JSON")
				}
				hasAuthenticationWarning := false
				for _, warning := range result.Warnings {
					if strings.Contains(warning, "Commands can still authenticate.") {
						hasAuthenticationWarning = true
					}
				}
				if hasAuthenticationWarning != tc.available {
					t.Errorf("logout preview=%v remaining-credentials warning = %v, want %v", preview, hasAuthenticationWarning, tc.available)
				}
				if result.Environment != alias || result.CredentialSource != authops.LogoutCredentialSourceOS || result.TableauPATRevoked {
					t.Error("logout changed the selected environment, credential source, or remote PAT state")
				}
				if preview {
					if result.Status != "preview" || result.Plan == nil || !result.Plan.StoredCredentialReferencePresent {
						t.Fatal("logout preview omitted the stored credential removal plan")
					}
					if result.Plan.EnvironmentCredentialsAvailable != tc.available {
						t.Errorf("preview credential availability = %v, want %v", result.Plan.EnvironmentCredentialsAvailable, tc.available)
					}
					after, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(before, after) || len(store.deleted) != 0 || len(store.records) != 1 {
						t.Fatal("logout preview changed configuration bytes or the fake credential store")
					}
				} else if result.Status != "removed" || result.Plan != nil || !slices.Equal(store.deleted, []coreauth.CredentialReference{reference}) || len(store.records) != 0 {
					t.Error("logout did not remove only the selected fake stored credential")
				}
			}
			if network.calls != 0 {
				t.Fatal("local logout attempted a remote request")
			}
			updated, err := config.Load(path)
			if err != nil {
				t.Fatal("logout left unreadable configuration")
			}
			entry, err := updated.EnvironmentForRepair(alias)
			if err != nil || entry.Auth.CredentialRef != "" || entry.Auth.PATNameEnv != tc.nameVariable || entry.Auth.PATSecretEnv != tc.secretVariable || entry.URL != "https://tableau.example.test" || entry.SiteContentURL != "fixture-site" || updated.DefaultEnvironment != alias {
				t.Fatal("logout changed environment settings beyond clearing the stored credential reference")
			}
		})
	}
}
