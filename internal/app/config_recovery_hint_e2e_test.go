package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
)

func TestMalformedCredentialLogoutRecoveryPreservesAliasAndConfig(t *testing.T) {
	for _, alias := range []string{"-broken", "broken $(not-a-command); quote's"} {
		t.Run(alias, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings with spaces.yaml")
			configuration := fmt.Sprintf("version: 1\nenvironments:\n  %q:\n    url: https://tableau.example.test\n    auth:\n      type: pat\n      credential_ref: %q\n", alias, resilienceRejectedValue)
			if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			store := &fakePATStore{}
			network := &warningTestNoNetwork{}
			options := Options{ConfigPath: filepath.Join(t.TempDir(), "ordinary.yaml"), PATStore: store, HTTPClient: &http.Client{Transport: network}}
			for _, preview := range []bool{true, false} {
				args := []string{"--config", path, "auth", "logout", "--environment=" + alias, "--json"}
				if preview {
					args = append(args, "--preview")
				}
				out := resilienceRequire(t, options, args...)
				var result struct {
					Help     []string `json:"help"`
					Warnings []string `json:"warnings"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				want := commandhint.Command("--config", path, "env", "remove", "--", alias)
				if !slices.Contains(result.Help, want) {
					t.Fatalf("logout recovery lacks the exact shell-safe config-bound command %q: %s", want, out)
				}
				if strings.Contains(strings.Join(result.Warnings, " "), "tadx env remove") || len(store.deleted) != 0 {
					t.Fatalf("logout emitted an unbound repair command or deleted an unlocatable credential: %s", out)
				}
				if network.calls != 0 {
					t.Fatal("local credential recovery attempted a remote request")
				}
			}
		})
	}
}

func TestLogoutRepairsExplicitEmptyAndWhitespaceAliases(t *testing.T) {
	for _, alias := range []string{"", "   "} {
		t.Run(fmt.Sprintf("alias_%q", alias), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "selected.yaml")
			const reference coreauth.CredentialReference = "cred_77777777777777777777777777777777"
			configuration := fmt.Sprintf("version: 1\ndefault_environment: good\nenvironments:\n  good:\n    url: https://good.example.test\n    auth:\n      type: pat\n  %q:\n    url: https://broken.example.test\n    auth:\n      type: pat\n      credential_ref: %s\n", alias, reference)
			if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}}}
			network := &warningTestNoNetwork{}
			options := Options{ConfigPath: filepath.Join(t.TempDir(), "ordinary.yaml"), PATStore: store, HTTPClient: &http.Client{Transport: network}}
			code, _ := resilienceRun(t, options, "--config", path, "auth", "logout", "--json")
			if code == 0 || len(store.deleted) != 0 {
				t.Fatal("omitted logout selector inferred an invalid or ordinary default environment")
			}
			for _, preview := range []bool{true, false} {
				args := []string{"--config", path, "auth", "logout", "--env=" + alias, "--json"}
				if preview {
					args = append(args, "--preview")
				}
				out := resilienceRequire(t, options, args...)
				if strings.Contains(out, "fixture-secret") || strings.Contains(out, string(reference)) {
					t.Fatal("logout exposed stored credential identity or values")
				}
				if preview && len(store.deleted) != 0 {
					t.Fatal("logout preview deleted a stored credential")
				}
			}
			if !slices.Equal(store.deleted, []coreauth.CredentialReference{reference}) {
				t.Fatal("logout did not delete only the exact invalid entry's stored credential")
			}
			if network.calls != 0 {
				t.Fatal("local credential cleanup attempted a remote request")
			}
			updated, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := updated.EnvironmentForRepair(alias)
			if err != nil || entry.Auth.CredentialRef != "" || updated.DefaultEnvironment != "good" {
				t.Fatal("logout did not clear the exact invalid reference while preserving default selection")
			}
		})
	}
}

func TestLogoutExplicitMissingBlankDoesNotInferSoleEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected.yaml")
	const reference coreauth.CredentialReference = "cred_88888888888888888888888888888888"
	configuration := fmt.Sprintf("version: 1\ndefault_environment: good\nenvironments:\n  good:\n    url: https://good.example.test\n    auth:\n      type: pat\n      credential_ref: %s\n", reference)
	if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}}}
	network := &warningTestNoNetwork{}
	options := Options{ConfigPath: path, PATStore: store, HTTPClient: &http.Client{Transport: network}}
	code, out := resilienceRun(t, options, "auth", "logout", "--env=", "--json")
	if code == 0 || len(store.deleted) != 0 || network.calls != 0 {
		t.Fatalf("explicit missing blank alias used the sole environment: code=%d output=%s", code, out)
	}
	resilienceRequire(t, options, "auth", "logout", "--json")
	if !slices.Equal(store.deleted, []coreauth.CredentialReference{reference}) || network.calls != 0 {
		t.Fatal("omitted logout selection lost sole-environment inference or attempted a remote request")
	}
}
