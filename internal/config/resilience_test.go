package config_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

const resilienceSettings = `version: 1
default_environment: broken
environments:
  good:
    url: https://good.example.test
    auth:
      type: pat
  broken:
    url: https://broken.example.test
    api_version: old-version
    auth:
      type: pat
      pat_secret_env: rejected-secret/value
`

func resilienceFile(t *testing.T, settings string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResilienceHealthySelectionSurvivesUnrelatedInvalidEntry(t *testing.T) {
	path := resilienceFile(t, resilienceSettings)
	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("entry problem rejects readable settings: %v", err)
	}
	if _, err := c.ResolveEnvironment("good"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveEnvironment(""); err == nil || !strings.Contains(err.Error(), "pat_secret_env") || strings.Contains(err.Error(), "rejected-secret/value") {
		t.Fatalf("default invalid entry error = %v", err)
	}
}

func TestResilienceUnrelatedWritePreservesInvalidEntry(t *testing.T) {
	path := resilienceFile(t, resilienceSettings)
	_, err := config.Update(path, false, func(c config.Config) (config.Config, error) {
		c.DefaultEnvironment = "good"
		return c, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "pat_secret_env: rejected-secret/value") || strings.Contains(string(data), "api_version") {
		t.Fatal("write loses quarantined content or retains the deleted API setting")
	}
}

func TestResilienceEntryDecodeFailureIsLocal(t *testing.T) {
	for _, field := range []string{"future_field: value", "cache_max_concurrency: [value]"} {
		t.Run(field, func(t *testing.T) {
			settings := strings.Replace(resilienceSettings, "api_version: old-version", field, 1)
			c, err := config.Load(resilienceFile(t, settings))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.ResolveEnvironment("good"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.ResolveEnvironment("broken"); err == nil {
				t.Fatal("malformed entry resolves")
			}
		})
	}
}

func TestResilienceSafetyAndSyntaxFailuresRemainGlobal(t *testing.T) {
	for _, settings := range []string{
		"version: 2\n",
		"version: [1]\n",
		"version: 1\nsite_mutations: invalid\n",
		"version: 1\nsite_mutations:\n  - server_url: https://example.test\n    enabled: true\n    future_permission: true\n",
		"version: 1\nsite_mutations:\n  - server_url: https://example.test\n    enabled: invalid\n",
		"version: 1\nversion: 1\n",
		"version: 1\n---\nversion: 1\n",
		"version: 1\nenvironments: [\n",
	} {
		if _, err := config.Load(resilienceFile(t, settings)); err == nil {
			t.Fatal("file-level failure loads")
		}
	}
}

func TestResilienceUnknownTopLevelPreservedWithoutValuesInWarnings(t *testing.T) {
	path := resilienceFile(t, "version: 1\nfuture_option:\n  nested: opaque-value\n")
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "nested: opaque-value") {
		t.Fatal("unknown top-level content is lost")
	}
}

func TestResilienceRepairPatchRequiresFullyValidEntry(t *testing.T) {
	path := resilienceFile(t, resilienceSettings)
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PatchEnvironment("broken", map[string]any{"url": "https://updated.example.test"}); err == nil {
		t.Fatal("partial repair accepts remaining violation")
	}
	if err := c.PatchEnvironment("broken", map[string]any{"auth.pat_secret_env": "VALID_NAME"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveEnvironment("broken"); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	if c, err := config.Load(path); err != nil || len(c.InvalidEnvironments) != 0 {
		t.Fatal("repaired entry remains invalid")
	}
}

func TestResiliencePatchPreservesUntouchedMalformedAndUnknownFields(t *testing.T) {
	for _, field := range []string{"future_field: opaque", "cache_max_concurrency: [opaque]"} {
		settings := strings.Replace(resilienceSettings, "api_version: old-version", field, 1)
		c, err := config.Load(resilienceFile(t, settings))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.PatchEnvironment("broken", map[string]any{"auth.pat_secret_env": "VALID_NAME"}); err == nil {
			t.Fatal("patch silently drops an untouched malformed or unknown field")
		}
		if _, err := c.ResolveEnvironment("broken"); err == nil {
			t.Fatal("failed patch changes the original entry")
		}
	}
}

func TestResilienceWriteNeverIntroducesInvalidity(t *testing.T) {
	for _, preview := range []bool{false, true} {
		path := resilienceFile(t, resilienceSettings)
		mutate := func(c config.Config) (config.Config, error) {
			c.Environments["another"] = config.Environment{URL: "invalid", Auth: config.Auth{Type: "pat"}}
			return c, nil
		}
		var err error
		if preview {
			_, err = config.PreviewUpdate(path, false, mutate)
		} else {
			_, err = config.Update(path, false, mutate)
		}
		if err == nil {
			t.Fatal("new invalid entry is accepted")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != resilienceSettings {
			t.Fatal("rejected mutation changes disk")
		}
	}
}

func TestResilienceStoredCredentialClearPreservesOtherInvalidFields(t *testing.T) {
	const reference = "cred_11111111111111111111111111111111"
	settings := strings.Replace(resilienceSettings, "pat_secret_env: rejected-secret/value", "credential_ref: "+reference+"\n      pat_secret_env: rejected-secret/value", 1)
	path := resilienceFile(t, settings)
	_, err := config.UpdateWithPostSave(path, false, func(c config.Config) (config.Config, func() error, error) {
		if err := c.ClearCredentialReference("broken"); err != nil {
			return c, nil, err
		}
		return c, func() error { return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.InvalidEnvironments) != 1 || c.InvalidEnvironments["broken"].CredentialRef != "" {
		t.Fatal("clearing credentials removes other quarantine")
	}
	if err := c.RemoveEnvironment("broken"); err != nil {
		t.Fatal(err)
	}
	c.DefaultEnvironment = "good"
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
}

func TestResilienceCredentialClearRollsBackOriginalInvalidEntry(t *testing.T) {
	settings := strings.Replace(resilienceSettings, "pat_secret_env: rejected-secret/value", "credential_ref: cred_11111111111111111111111111111111\n      pat_secret_env: rejected-secret/value", 1)
	path := resilienceFile(t, settings)
	_, err := config.UpdateWithPostSave(path, false, func(c config.Config) (config.Config, func() error, error) {
		if err := c.ClearCredentialReference("broken"); err != nil {
			return c, nil, err
		}
		return c, func() error { return errors.New("external removal failed") }, nil
	})
	if err == nil {
		t.Fatal("external failure is lost")
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.InvalidEnvironments["broken"].CredentialRef == "" {
		t.Fatal("rollback loses original stored credential")
	}
}

func TestResilienceCrossEntryCollisionsMarkEveryParticipant(t *testing.T) {
	settings := `version: 1
environments:
  good:
    url: https://good.example.test
    auth: {type: pat}
`
	for _, alias := range []string{"first", "second", "third"} {
		settings += "  " + alias + ":\n    url: https://example.test\n    auth: {type: pat, credential_ref: cred_11111111111111111111111111111111}\n"
	}
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.InvalidEnvironments) != 3 || len(c.Environments) != 1 {
		t.Fatal("cross-entry collision misses participants")
	}
	for _, alias := range []string{"first", "second", "third"} {
		invalid := c.InvalidEnvironments[alias]
		if len(invalid.Violations) != 2 {
			t.Fatal("collision does not name every counterpart")
		}
		for _, violation := range invalid.Violations {
			if violation.OtherEntry == "" || violation.OtherEntry == alias {
				t.Fatal("collision lacks counterpart")
			}
		}
	}
	if _, err := c.ResolveWriteEnvironment(""); err == nil || !strings.Contains(err.Error(), "Multiple") {
		t.Fatal("quarantine hides aliases from write selection")
	}
	if err := c.RemoveEnvironment("first"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveEnvironment("second"); err != nil {
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
	if len(c.InvalidEnvironments) != 0 {
		t.Fatal("remaining collision participant is not promoted")
	}
}

func TestResilienceWarningsAndTypedErrorsNeverExposeRejectedValues(t *testing.T) {
	c, err := config.Load(resilienceFile(t, resilienceSettings))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ResolveEnvironment("broken")
	invalid, ok := errors.AsType[*config.InvalidEnvironmentError](err)
	if !ok {
		t.Fatal("selection does not preserve typed invalid error")
	}
	data, marshalErr := json.Marshal(c.ConfigurationWarnings())
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, text := range []string{string(data), err.Error(), invalid.CorrectiveAction()} {
		if strings.Contains(text, "rejected-secret/value") {
			t.Fatal("diagnostic exposes rejected value")
		}
	}
	if len(c.ConfigurationWarnings()) != 1 || !strings.Contains(string(data), "revoked") {
		t.Fatal("warning omits risk or repeats entry")
	}
	data, marshalErr = json.Marshal(c)
	if marshalErr != nil || strings.Contains(string(data), "rejected-secret/value") {
		t.Fatal("configuration JSON exposes quarantined content")
	}
}

func TestResilienceNewCrossCollisionWithUntouchedInvalidEntryRejected(t *testing.T) {
	settings := strings.Replace(resilienceSettings, "pat_secret_env: rejected-secret/value", "credential_ref: cred_11111111111111111111111111111111\n      pat_secret_env: rejected-secret/value", 1)
	path := resilienceFile(t, settings)
	_, err := config.Update(path, false, func(c config.Config) (config.Config, error) {
		c.Environments["another"] = config.Environment{URL: "https://example.test", Auth: config.Auth{Type: "pat", CredentialRef: "cred_11111111111111111111111111111111"}}
		return c, nil
	})
	if err == nil {
		t.Fatal("write introduces collision with quarantined identity")
	}
}

func TestResilienceMissingDefaultsOnlyFailSelection(t *testing.T) {
	settings := strings.Replace(resilienceSettings, "default_environment: broken", "default_environment: missing\ndefault_workspace: missing", 1)
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveEnvironment("good"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveEnvironment(""); err == nil {
		t.Fatal("missing default resolves")
	}
	if _, _, err := c.ResolveWorkspace(""); err == nil {
		t.Fatal("missing workspace default resolves")
	}
	if err := config.Save(filepath.Join(t.TempDir(), "settings.yaml"), c); err != nil {
		t.Fatal("unrelated write validates unchanged broken defaults")
	}
	c.DefaultEnvironment = "another_missing"
	if err := config.Save(filepath.Join(t.TempDir(), "settings.yaml"), c); err == nil {
		t.Fatal("changed default points to missing entry")
	}
}

func TestResilienceWorkspaceQuarantineRetainsKnownRoots(t *testing.T) {
	settings := `version: 1
workspaces:
  broken:
    id: [invalid]
    path: child/root
    future_field: opaque
  good:
    id: ws_11111111111111111111111111111111
    path: valid/root
`
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	if c.WorkspaceRegistrations()["broken"].Path != "child/root" {
		t.Fatal("quarantine hides a known filesystem root")
	}
	if _, _, err := c.ResolveWorkspace("good"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ResolveWorkspace("broken"); err == nil {
		t.Fatal("invalid registration resolves")
	}
	if err := c.RemoveWorkspace("broken"); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(filepath.Join(t.TempDir(), "settings.yaml"), c); err != nil {
		t.Fatal(err)
	}
}

func TestResilienceInvalidEntryPreservesLegacyDefaultOnUnrelatedWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "legacy-root")
	settings := strings.Replace(resilienceSettings, "api_version: old-version", "default_workspace: "+quoteYAML(root), 1)
	path := resilienceFile(t, settings)
	_, err := config.Update(path, false, func(c config.Config) (config.Config, error) { c.DefaultEnvironment = "good"; return c, nil })
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "default_workspace: "+quoteYAML(root)) {
		t.Fatal("unrelated write migrates a quarantined entry")
	}
}

func TestResilienceSyntaxDiagnosticsNeverExposeAliasValues(t *testing.T) {
	path := resilienceFile(t, "version: 1\nenvironments:\n  broken: *REJECTED_ALIAS_VALUE\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("unknown alias loads")
	}
	if strings.Contains(err.Error(), "REJECTED_ALIAS_VALUE") {
		t.Fatal("syntax error exposes rejected alias value")
	}
}

func TestResilienceTypedReplacementCannotReuseQuarantinePermission(t *testing.T) {
	c, err := config.Load(resilienceFile(t, resilienceSettings))
	if err != nil {
		t.Fatal(err)
	}
	environment, err := c.EnvironmentForRepair("broken")
	if err != nil {
		t.Fatal(err)
	}
	// A typed replacement is a changed entry, even when it reconstructs the old values.
	c.Environments["broken"] = environment
	if err := config.Save(filepath.Join(t.TempDir(), "settings.yaml"), c); err == nil {
		t.Fatal("typed invalid replacement reuses unchanged quarantine permission")
	}
}

func TestResilienceRecoveryKeepsNamesAsStructuredArguments(t *testing.T) {
	for _, name := range []string{"good", "team workspace", `one' " $HOME; &`, "--preview"} {
		environment := &config.InvalidEnvironmentError{Alias: name, Violations: []config.EntryViolation{{Field: "pat_secret_env", Rule: "invalid", SecretRisk: true}}}
		commands := environment.CorrectiveCommands()
		if len(commands) != 3 {
			t.Fatal("environment recovery commands are incomplete")
		}
		for _, command := range commands {
			if command[len(command)-1] != name {
				t.Fatal("recovery modifies the logical name")
			}
			if name == "--preview" && command[0] == "env" && command[len(command)-2] != "--" {
				t.Fatal("positional option-shaped alias lacks delimiter")
			}
		}
		if !strings.Contains(environment.CorrectiveExplanation(), "revoked") {
			t.Fatal("structured recovery loses revoke advice")
		}
		if environment.CorrectiveAction() != "Run tadx env list to inspect configured entries, then correct or remove the invalid environment." {
			t.Fatal("fallback embeds an unquoted identity")
		}
		workspace := &config.InvalidWorkspaceError{Name: name, Violations: []config.EntryViolation{{Field: "path", Rule: "invalid"}}}
		workspaceCommands := workspace.CorrectiveCommands()
		if len(workspaceCommands) != 2 {
			t.Fatal("workspace recovery lacks registration")
		}
		registration := workspaceCommands[1]
		registrationName := name
		if config.ValidateWorkspaceName(name) != nil {
			registrationName = "<name>"
		}
		if len(registration) < 5 || registration[0] != "workspace" || registration[1] != "register" || registration[2] != "--path" || registration[3] != "<path>" || registration[len(registration)-1] != registrationName {
			t.Fatal("registration recovery does not match CLI syntax")
		}
		if name == "--preview" && registration[len(registration)-2] != "--" {
			t.Fatal("registration recovery lacks positional delimiter after flags")
		}
		if !strings.Contains(workspace.CorrectiveExplanation(), "first, then") || !strings.Contains(workspace.CorrectiveExplanation(), "Replace <path>") {
			t.Fatal("workspace repair does not describe sequence and placeholder")
		}
		command := workspaceCommands[0]
		if command[len(command)-1] != name {
			t.Fatal("workspace recovery changes its logical name")
		}
		if name == "--preview" && command[len(command)-2] != "--" {
			t.Fatal("option-shaped workspace lacks delimiter")
		}
		if workspace.CorrectiveAction() != "Run tadx workspace list to inspect registrations, then repair the invalid workspace registration." {
			t.Fatal("workspace fallback embeds an unquoted identity")
		}
	}
}

func TestResilienceDuplicateUnrelatedFieldsRetainUniqueCredentialIdentity(t *testing.T) {
	const reference = "cred_11111111111111111111111111111111"
	settings := `version: 1
environments:
  broken:
    url: https://one.example.test
    url: https://two.example.test
    auth:
      type: pat
      credential_ref: ` + reference + `
  other:
    url: https://other.example.test
    auth:
      type: pat
      credential_ref: ` + reference + `
`
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	if c.InvalidEnvironments["broken"].CredentialRef != reference {
		t.Fatal("duplicate URL discards a unique stored credential identity")
	}
	if len(c.InvalidEnvironments) != 2 {
		t.Fatal("partial credential identity is absent from cross-entry collision validation")
	}
	environment, err := c.EnvironmentForRepair("broken")
	if err != nil || environment.Auth.CredentialRef != reference {
		t.Fatal("repair cannot locate the uniquely configured credential")
	}
	if environment.URL != "" {
		t.Fatal("duplicate URL is guessed")
	}
	if err := c.ClearCredentialReference("broken"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	c, err = config.Load(path)
	if err != nil || c.InvalidEnvironments["broken"].CredentialRef != "" || len(c.InvalidEnvironments) != 1 {
		t.Fatal("credential clearing does not preserve unrelated quarantine and release the other participant")
	}
}

func TestResilienceDuplicateUnrelatedWorkspaceFieldRetainsUniqueRootAndIdentity(t *testing.T) {
	settings := `version: 1
workspaces:
  broken:
    id: ws_11111111111111111111111111111111
    path: child/root
    future_field: first
    future_field: second
  other:
    id: ws_11111111111111111111111111111111
    path: child/root
`
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	registration := c.WorkspaceRegistrations()["broken"]
	if registration.ID != "ws_11111111111111111111111111111111" || registration.Path != "child/root" {
		t.Fatal("unrelated duplicate workspace field hides a unique identity or filesystem root")
	}
	if len(c.InvalidWorkspaces) != 2 {
		t.Fatal("quarantined workspace identities are absent from cross-entry collision validation")
	}
	_, repaired, err := c.WorkspaceForRepair("broken")
	if err != nil || repaired != registration {
		t.Fatal("workspace repair loses known fields")
	}
}

func TestResilienceAmbiguousCredentialAndWorkspaceIdentitiesAreNeverGuessed(t *testing.T) {
	for _, auth := range []string{
		"    auth:\n      type: pat\n      credential_ref: cred_11111111111111111111111111111111\n      credential_ref: cred_22222222222222222222222222222222\n",
		"    auth: {type: pat, credential_ref: cred_11111111111111111111111111111111}\n    auth: {type: pat, credential_ref: cred_22222222222222222222222222222222}\n",
	} {
		settings := "version: 1\nenvironments:\n  broken:\n    url: https://example.test\n" + auth
		c, err := config.Load(resilienceFile(t, settings))
		if err != nil {
			t.Fatal(err)
		}
		environment, err := c.EnvironmentForRepair("broken")
		if err != nil || environment.Auth.CredentialRef != "" || c.InvalidEnvironments["broken"].CredentialRef != "" {
			t.Fatal("ambiguous stored credential identity is guessed")
		}
	}
	settings := "version: 1\nworkspaces:\n  broken:\n    id: ws_11111111111111111111111111111111\n    path: first/root\n    path: second/root\n"
	c, err := config.Load(resilienceFile(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	if c.WorkspaceRegistrations()["broken"].Path != "" {
		t.Fatal("ambiguous workspace path is guessed")
	}
}

func TestResilienceExplicitPatchReplacesEveryOccurrenceOfSelectedField(t *testing.T) {
	settings := `version: 1
environments:
  broken:
    url: https://first.example.test
    url: https://second.example.test
    url: https://third.example.test
    auth: {type: pat}
`
	path := resilienceFile(t, settings)
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PatchEnvironment("broken", map[string]any{"url": "https://corrected.example.test"}); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "url:") != 1 || !strings.Contains(string(data), "https://corrected.example.test") {
		t.Fatal("explicit patch does not replace every old field occurrence")
	}
}

func TestResilienceExplicitPatchPreservesOtherAmbiguity(t *testing.T) {
	for _, other := range []string{
		"    future_field: first\n    future_field: second\n    auth: {type: pat}\n",
		"    auth: {type: pat}\n    auth: {type: pat}\n",
	} {
		settings := "version: 1\nenvironments:\n  broken:\n    url: https://first.example.test\n    url: https://second.example.test\n" + other
		path := resilienceFile(t, settings)
		c, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.PatchEnvironment("broken", map[string]any{"url": "https://corrected.example.test"}); err == nil {
			t.Fatal("patch drops unrelated ambiguity")
		}
		if err := config.Save(path, c); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "https://first.example.test") || !strings.Contains(string(data), "https://second.example.test") {
			t.Fatal("failed patch changes original entry")
		}
	}
}

func TestResilienceDuplicateMalformedLegacyAPIVersionsAreIgnoredAndAllRemoved(t *testing.T) {
	settings := `version: 1
environments:
  good:
    url: https://good.example.test
    api_version: [opaque]
    api_version: {nested: opaque}
    auth: {type: pat}
  broken:
    url: https://broken.example.test
    api_version: [opaque]
    api_version: {nested: opaque}
    auth: {type: pat, pat_secret_env: rejected-secret/value}
`
	path := resilienceFile(t, settings)
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveEnvironment("good"); err != nil {
		t.Fatal("duplicate legacy setting invalidates healthy environment")
	}
	for _, violation := range c.InvalidEnvironments["broken"].Violations {
		if violation.Field == "api_version" {
			t.Fatal("legacy setting contributes entry violations")
		}
	}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "api_version") {
		t.Fatal("write retains a duplicated legacy API setting")
	}
}
