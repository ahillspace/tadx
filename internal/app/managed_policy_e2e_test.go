package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/lastcommand"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

type fixtureManagedPolicy struct {
	state    string
	allowed  map[string]bool
	remote   bool
	warnings []string
	checks   []managedpolicy.ProtectionCheck
}

type invalidLocatorPolicy struct{ fixtureManagedPolicy }

func (invalidLocatorPolicy) Status() managedpolicy.Status {
	return managedpolicy.Status{State: managedpolicy.StateError, Reason: "invalid protected policy locator"}
}

func TestManagedPolicySamplesRemainAvailableWithInvalidLocator(t *testing.T) {
	options := overviewOptions(t, t.TempDir())
	options.managedPolicy = invalidLocatorPolicy{fixtureManagedPolicy{state: managedpolicy.StateError}}
	directory := filepath.Join(t.TempDir(), "samples")
	var stdout bytes.Buffer
	if code := Run(t.Context(), []string{"policy", "samples", "--output", directory, "--json"}, &stdout, options); code != 0 {
		t.Fatalf("recovery samples unavailable: code=%d output=%s", code, &stdout)
	}
	var result struct {
		Status       string   `json:"status"`
		SystemPath   string   `json:"system_path"`
		Files        []string `json:"files"`
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "created" || result.SystemPath != "" || len(result.Files) != 3 || !strings.Contains(strings.Join(result.Instructions, " "), "locator") {
		t.Fatalf("recovery output misrepresented unavailable location: %+v", result)
	}
	for _, path := range result.Files {
		if _, err := os.Stat(filepath.FromSlash(path)); err != nil {
			t.Fatal(err)
		}
	}
	if status := options.managedPolicy.Status(); status.State != managedpolicy.StateError || status.Path != "" {
		t.Fatalf("sample recovery changed invalid locator state: %+v", status)
	}
}

func (p fixtureManagedPolicy) Status() managedpolicy.Status {
	return managedpolicy.Status{State: p.state, Path: "/fixed/system/policy.json", Protected: p.state == managedpolicy.StateActive, RemoteMutations: p.remote, Warnings: p.warnings, Checks: p.checks}
}

func TestManagedPolicyAncestorWarningOnlyInStatusOutput(t *testing.T) {
	options := overviewOptions(t, t.TempDir())
	options.ConfigPath = authConfig(t, "")
	warning := "Managed policy path warning: policy substitution may be possible."
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"env.profile.list": true}, warnings: []string{warning}, checks: []managedpolicy.ProtectionCheck{{Path: "/fixed", Kind: "ancestor-owner-acl-and-links", Passed: false, Reason: "writable ancestor"}}}
	var stdout, stderr bytes.Buffer
	options.Stderr = &stderr
	if code := Run(t.Context(), []string{"env", "list", "--json"}, &stdout, options); code != 0 {
		t.Fatalf("allowed command blocked: %d %s", code, &stdout)
	}
	if !json.Valid(stdout.Bytes()) || stderr.Len() != 0 {
		t.Fatalf("allowed command streams: stdout=%s stderr=%s", &stdout, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(t.Context(), []string{"admin", "user", "list", "--env", "dev", "--json"}, &stdout, options); code == 0 || !strings.Contains(stdout.String(), "policy.denied") || stderr.Len() != 0 {
		t.Fatalf("denied command streams: stdout=%s stderr=%s", &stdout, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(t.Context(), []string{"policy", "status", "--full", "--json"}, &stdout, options); code != 0 || stderr.Len() != 0 {
		t.Fatalf("policy status streams: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	var status struct {
		Warnings []string                        `json:"warnings"`
		Checks   []managedpolicy.ProtectionCheck `json:"protection_checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil || len(status.Warnings) != 1 || status.Warnings[0] != warning || len(status.Checks) != 1 || status.Checks[0].Kind != "ancestor-owner-acl-and-links" {
		t.Fatalf("policy status diagnostics: status=%+v err=%v output=%s", status, err, &stdout)
	}
}
func (p fixtureManagedPolicy) CheckCapability(id string) error {
	if p.state == managedpolicy.StateError {
		return managedpolicy.ErrBlocked
	}
	if !p.allowed[id] {
		return managedpolicy.ErrCapabilityDenied
	}
	return nil
}
func (p fixtureManagedPolicy) CheckRemoteMutation() error {
	if p.state == managedpolicy.StateError {
		return managedpolicy.ErrBlocked
	}
	if !p.remote {
		return managedpolicy.ErrRemoteMutationDenied
	}
	return nil
}

func TestManagedPolicyDeniesOperationalVariantsBeforeRequests(t *testing.T) {
	for _, args := range [][]string{
		{"admin", "user", "list", "--env", "dev"},
		{"adm", "usr", "ls", "--env", "dev"},
		{"admin", "user", "list", "--env", "dev", "--cache"},
		{"admin", "user", "delete", "--env", "dev", "--id", "user-1", "--preview"},
		{"admin", "user", "delete", "--env", "dev", "--id", "user-1", "--id", "user-2", "--preview"},
		{"content", "workbook", "publish", "--env", "dev", "--file", "unused.twb", "--project-id", "project-1"},
		{}, {"--version"}, {"-v"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root := t.TempDir()
			options := overviewOptions(t, root)
			options.ConfigPath = authConfig(t, "")
			options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateError}
			options.PublicationWorkers = true
			options.WorkerLauncher = func(_ context.Context, _, _ string) error { t.Fatal("denied operation launched worker"); return nil }
			var out bytes.Buffer
			if code := Run(t.Context(), append(args, "--json"), &out, options); code == 0 || !strings.Contains(out.String(), "policy.denied") || !strings.Contains(out.String(), `"outcome":"not_attempted"`) {
				t.Fatalf("code=%d output=%s", code, &out)
			}
			if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
				if !strings.Contains(out.String(), `"operation":"version.get"`) {
					t.Fatalf("wrong version policy ID: %s", &out)
				}
			}
		})
	}
}

func TestManagedPolicyBatchFilesAndEnvironmentOverridesCannotBypass(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	options.ConfigPath = authConfig(t, "")
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"env.profile.list": true}}
	t.Setenv("TADX_POLICY_PATH", filepath.Join(root, "ignored-policy.json"))
	t.Setenv("TADX_ENABLE_MUTATIONS", "1")
	file := filepath.Join(root, "items.json")
	if err := os.WriteFile(file, []byte(`{"items":[{"id":"user-1"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Run(t.Context(), []string{"admin", "user", "delete", "--env", "dev", "--batch-file", file, "--preview", "--json"}, &out, options); code == 0 || !strings.Contains(out.String(), "policy.denied") {
		t.Fatalf("batch bypass: code=%d %s", code, &out)
	}
	out.Reset()
	if code := Run(t.Context(), []string{"env", "list", "--json"}, &out, options); code != 0 {
		t.Fatalf("allowed local operation blocked: %d %s", code, &out)
	}
}

func TestManagedPolicyPreviewNeedsCapabilityButNotRemotePermission(t *testing.T) {
	options := overviewOptions(t, t.TempDir())
	options.ConfigPath = authConfig(t, "")
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"job.cancel": true}}
	t.Setenv("TADX_DEV_PAT_NAME", "")
	t.Setenv("TADX_DEV_PAT_SECRET", "")
	for _, preview := range []bool{false, true} {
		args := []string{"job", "cancel", "--env", "dev", "--id", "job-1", "--json"}
		if preview {
			args = append(args, "--preview")
		}
		var out bytes.Buffer
		code := Run(t.Context(), args, &out, options)
		if code == 0 {
			t.Fatal("fixture unexpectedly authenticated")
		}
		if denied := strings.Contains(out.String(), "policy.denied"); denied == preview {
			t.Fatalf("preview=%v wrong policy result: %s", preview, &out)
		}
		if preview && !strings.Contains(out.String(), "job.cancel.setup") {
			t.Fatalf("allowed preview did not reach authentication setup: %s", &out)
		}
	}
}

func TestManagedPolicyRecoveryToolsRemainAvailable(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateError}
	directory := filepath.Join(root, "samples")
	for _, args := range [][]string{{"--help"}, {"policy"}, {"admin", "user"}, {"help", "admin", "user", "list"}, {"policy", "--help"}, {"policy", "status"}, {"policy", "samples", "--output", directory}, {"policy", "validate", filepath.Join(directory, "read-only.json")}} {
		var out bytes.Buffer
		if code := Run(t.Context(), append(args, "--json"), &out, options); code != 0 {
			t.Fatalf("%v code=%d output=%s", args, code, &out)
		}
	}
	before, err := os.ReadFile(filepath.Join(directory, "superuser.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Run(t.Context(), []string{"policy", "samples", "--output", directory, "--json"}, &out, options); code == 0 {
		t.Fatal("sample files overwritten")
	}
	after, err := os.ReadFile(filepath.Join(directory, "superuser.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing candidate changed")
	}
	if _, err := os.Stat(filepath.Join(directory, "admin.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy sample must not be advertised: %v", err)
	}
	for _, name := range []string{"read-only", "read-write-no-admin", "superuser"} {
		data, err := os.ReadFile(filepath.Join(directory, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := managedpolicy.Parse(data, capability.All())
		if err != nil {
			t.Fatal(err)
		}
		if doc.RemoteMutations != (name != "read-only") {
			t.Fatalf("wrong remote ceiling for %s", name)
		}
		for _, id := range doc.AllowedCapabilities {
			definition, _ := capability.Lookup(id)
			if name == "read-write-no-admin" && definition.Administrative && definition.RemoteMutation {
				t.Fatalf("nonadmin sample includes administrative mutation %s", id)
			}
		}
	}
}

func TestManagedPolicyBlocksSavedAdministrativeResult(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"last": true}}
	store := lastcommand.Store{Path: filepath.Join(root, "last-result.json")}
	if err := store.Save(t.Context(), value.SavedExecution{RecordedAt: time.Now(), Operation: "admin.user.list", Result: json.RawMessage(`{"sensitive":"admin-private-result"}`)}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Run(t.Context(), []string{"last", "--json"}, &out, options); code == 0 || strings.Contains(out.String(), "admin-private-result") || !strings.Contains(out.String(), "policy") {
		t.Fatalf("saved result leaked: %d %s", code, &out)
	}
}

func TestManagedPolicyWorkerRechecksRestriction(t *testing.T) {
	root := t.TempDir()
	options := overviewOptions(t, root)
	options.ConfigPath = authConfig(t, "")
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"workbook.publish": true}, remote: false}
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish", ConfigPath: options.ConfigPath, Args: []string{"content", "workbook", "publish", "--env", "dev", "--file", "unused.twb", "--project-id", "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if code := runPublicationWorker(t.Context(), store.Directory, record.ID, options); code == 0 {
		t.Fatal("worker bypassed current ceiling")
	}
	result, err := store.Read(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.FullResult), "policy.denied") {
		t.Fatalf("missing worker denial: %s", result.FullResult)
	}
}

func TestManagedPolicyDiscoveryRetainsDeniedAvailability(t *testing.T) {
	r := &runtimeDependencies{managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"capability.get": true}, remote: true}}
	item, ok := (registrySource{runtime: r}).Get(t.Context(), "admin.user.list")
	if !ok || !item.PolicyDenied || item.ExecutionEnabled || item.PolicyReason == "" {
		t.Fatalf("denied discovery lost: %+v", item)
	}
	if !errors.Is(r.checkManagedCapability("admin.user.list"), managedpolicy.ErrCapabilityDenied) {
		t.Fatal("structured cause lost")
	}
	options := overviewOptions(t, t.TempDir())
	options.managedPolicy = r.managedPolicy
	var out bytes.Buffer
	if code := Run(t.Context(), []string{"capability", "get", "admin.user.list", "--json"}, &out, options); code != 0 {
		t.Fatalf("discovery failed: %d %s", code, &out)
	}
	var result struct {
		Capability struct {
			ExecutionEnabled bool `json:"execution_enabled"`
			PolicyDenied     bool `json:"policy_denied"`
			Administrative   bool `json:"administrative"`
		} `json:"capability"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Capability.ExecutionEnabled || !result.Capability.PolicyDenied || !result.Capability.Administrative {
		t.Fatalf("action overwrote policy denial: %s", &out)
	}
}
