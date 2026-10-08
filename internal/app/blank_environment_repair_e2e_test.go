package app

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

func TestBlankEnvironmentAliasesCanBeInspectedAndRemovedThroughCLI(t *testing.T) {
	for _, alias := range []string{"", " "} {
		t.Run(strconv.Quote(alias), func(t *testing.T) {
			path := resilienceConfig(t, "")
			resilienceReplace(t, path, "  broken:\n", "  "+strconv.Quote(alias)+":\n")
			options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
			out := resilienceRequire(t, options, "env", "list", "--json", "--full")
			var listing struct {
				Environments []struct {
					Alias  string `json:"alias"`
					Status string `json:"status"`
				} `json:"environments"`
			}
			if err := json.Unmarshal([]byte(out), &listing); err != nil || !slices.ContainsFunc(listing.Environments, func(entry struct {
				Alias  string `json:"alias"`
				Status string `json:"status"`
			}) bool {
				return entry.Alias == alias && entry.Status == "invalid"
			}) {
				t.Fatalf("blank entry was not visible as invalid: output=%s err=%v", out, err)
			}
			before := resilienceRawEnvironment(t, path, "good")
			out = resilienceRequire(t, options, "env", "get", "--json", "--", alias)
			if !strings.Contains(out, `"status":"invalid"`) {
				t.Fatalf("blank alias inspection selected another entry: %s", out)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			code, out := resilienceRun(t, options, "env", "update", "--pat-secret-env", "VALID_NAME", "--json", "--", alias)
			if code == 0 || !strings.Contains(out, "alias must not be empty") {
				t.Fatalf("blank alias update did not report remaining entry violation: %s", out)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("partial blank alias repair changed configuration")
			}
			resilienceRequire(t, options, "env", "remove", "--preview", "--json", "--", alias)
			resilienceRequire(t, options, "env", "remove", "--json", "--", alias)
			cfg, err := config.Load(path)
			if err != nil || !slices.Equal(cfg.EnvironmentAliases(), []string{"good"}) {
				t.Fatalf("blank alias removal changed another entry: aliases=%q err=%v", cfg.EnvironmentAliases(), err)
			}
			if current := resilienceRawEnvironment(t, path, "good"); !equalYAMLMeaning(before, current) {
				t.Fatal("blank alias removal changed unrelated environment")
			}
			if code, out := resilienceRun(t, options, "env", "get", "--json", "--", alias); code == 0 {
				t.Fatalf("missing blank alias inspection selected the default: %s", out)
			}
			for _, args := range [][]string{
				{"env", "get", "--json"},
				{"env", "remove", "--json"},
				{"env", "update", "--pat-secret-env", "VALID_NAME", "--json"},
				{"env", "add", "--url", "https://another.example.test", "--json", "--", alias},
				{"env", "set-default", "--json", "--", alias},
			} {
				if code, out := resilienceRun(t, options, args...); code != 2 {
					t.Fatalf("missing selection or blank creation/default was accepted: args=%q code=%d output=%s", args, code, out)
				}
			}
		})
	}
}

func equalYAMLMeaning(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func TestBlankEnvironmentAliasesKeepStoredPATRemovalGuardThroughCLI(t *testing.T) {
	const reference = coreauth.CredentialReference("cred_66666666666666666666666666666666")
	for _, alias := range []string{"", " "} {
		t.Run(strconv.Quote(alias), func(t *testing.T) {
			path := resilienceConfig(t, "      credential_ref: "+string(reference)+"\n")
			resilienceReplace(t, path, "  broken:\n", "  "+strconv.Quote(alias)+":\n")
			store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}}}
			options := Options{ConfigPath: path, PATStore: store}
			code, out := resilienceRun(t, options, "env", "remove", "--json", "--", alias)
			if code == 0 || !strings.Contains(out, "stored PAT") || len(store.deleted) != 0 {
				t.Fatalf("blank alias removal bypassed the stored PAT guard: %s", out)
			}
			cfg, err := config.Load(path)
			if err != nil || !slices.Contains(cfg.EnvironmentAliases(), alias) {
				t.Fatal("stored PAT guard removed blank entry", err)
			}
		})
	}
}
