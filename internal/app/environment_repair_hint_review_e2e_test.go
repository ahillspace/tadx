package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/commandhint"
)

func TestInvalidEnvironmentGetHelpRepairsOptionShapedAliasThroughCLI(t *testing.T) {
	const alias = "-broken alias'fixture"
	path := resilienceConfig(t, "")
	resilienceReplace(t, path, "  broken:\n", "  \""+alias+"\":\n")
	options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
	out := resilienceRequire(t, options, "env", "list", "--json")
	var result struct {
		Help []string `json:"help"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Help) != 1 {
		t.Fatalf("environment list omitted inspection help: output=%s err=%v", out, err)
	}
	inspection := parseCacheRecoveryCommand(t, result.Help[0])
	if want := []string{"--config", path, "env", "get", "--", alias}; !slices.Equal(inspection, want) {
		t.Fatalf("environment list emitted unusable inspection arguments: got=%q want=%q", inspection, want)
	}
	inspection = append(slices.Clone(inspection[:4]), append([]string{"--json"}, inspection[4:]...)...)
	out = resilienceRequire(t, options, inspection...)
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Help) != 2 {
		t.Fatalf("invalid environment omitted repair help: output=%s err=%v", out, err)
	}
	repairs := slices.Clone(result.Help)
	for index, verb := range []string{"update", "remove"} {
		args := parseCacheRecoveryCommand(t, repairs[index])
		want := []string{"--config", path, "env", verb, "--", alias}
		if !slices.Equal(args, want) {
			t.Fatalf("invalid environment emitted unusable repair arguments: got=%q want=%q", args, want)
		}
		if verb == "update" {
			delimiter := slices.Index(args, "--")
			patched := append(slices.Clone(args[:delimiter]), "--pat-secret-env", "VALID_NAME", "--json")
			args = append(patched, args[delimiter:]...)
		}
		out = resilienceRequire(t, options, args...)
		if verb == "update" {
			if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Help) != 1 {
				t.Fatalf("repaired environment omitted inspection help: output=%s err=%v", out, err)
			}
			inspection := parseCacheRecoveryCommand(t, result.Help[0])
			if want := []string{"--config", path, "env", "get", "--", alias}; !slices.Equal(inspection, want) {
				t.Fatalf("repaired environment emitted unusable inspection arguments: got=%q want=%q", inspection, want)
			}
			resilienceRequire(t, options, inspection...)
		}
	}
}

func TestStoredPATEnvironmentGuardRecoveryPreservesAliasAndConfigThroughCLI(t *testing.T) {
	const reference coreauth.CredentialReference = "cred_88888888888888888888888888888888"
	for _, alias := range []string{"broken alias'fixture", "-broken alias'fixture", "", " "} {
		t.Run(alias, func(t *testing.T) {
			path := resilienceConfig(t, "      credential_ref: "+string(reference)+"\n")
			resilienceReplace(t, path, "  broken:\n", "  \""+alias+"\":\n")
			store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}}}
			options := Options{ConfigPath: path, PATStore: store}
			for _, args := range [][]string{
				{"env", "remove", "--json", "--", alias},
				{"env", "update", "--url", "https://another.example.test", "--json", "--", alias},
			} {
				code, out := resilienceRun(t, options, args...)
				var result struct {
					Error struct {
						CorrectiveAction string `json:"corrective_action"`
					} `json:"error"`
				}
				if code == 0 || json.Unmarshal([]byte(out), &result) != nil {
					t.Fatalf("stored PAT guard failed: %s", out)
				}
				command := commandhint.Command("--config", path, "auth", "logout", "--environment", alias)
				if !strings.Contains(result.Error.CorrectiveAction, command) || len(store.deleted) != 0 {
					t.Fatalf("stored PAT guard omitted exact config-bound recovery %q: %s", command, out)
				}
				recovery := parseCacheRecoveryCommand(t, command)
				if want := []string{"--config", path, "auth", "logout", "--environment", alias}; !slices.Equal(recovery, want) {
					t.Fatalf("stored PAT guard recovery changed alias: got=%q want=%q", recovery, want)
				}
			}
		})
	}
}
