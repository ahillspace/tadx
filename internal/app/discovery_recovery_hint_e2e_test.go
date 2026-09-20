package app

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/toon"
	"github.com/ahillspace/tadx/internal/value"
)

func TestCapabilityListHintsPreserveSelectedSiteConsentThroughCLI(t *testing.T) {
	t.Setenv("TADX_ENVIRONMENT", "")
	options := overviewOptions(t, t.TempDir())
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, remote: true, allowed: map[string]bool{
		"capability.list": true, "capability.get": true, "project.create": true, "project.delete": true,
	}}
	selectedConfig := filepath.Join(t.TempDir(), "selected config's.yaml")
	cfg := config.Config{Version: 1, DefaultEnvironment: "default", Environments: map[string]config.Environment{
		"default": {URL: "https://tableau.example.com", SiteContentURL: "default", Auth: config.Auth{Type: "pat"}},
		"qa":      {URL: "https://tableau.example.com", SiteContentURL: "qa", Auth: config.Auth{Type: "pat"}},
	}, SiteMutations: []config.SiteMutation{
		{ServerURL: "https://tableau.example.com", SiteContentURL: "default", Enabled: false},
		{ServerURL: "https://tableau.example.com", SiteContentURL: "qa", Enabled: true},
	}}
	if err := config.Save(selectedConfig, cfg); err != nil {
		t.Fatal(err)
	}
	// The fallback has the same aliases but no consent for either site.
	cfg.SiteMutations = nil
	if err := config.Save(options.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"--config", selectedConfig, "capability", "list", "--environment", "qa", "--domain", "content", "--resource", "project", "--owner", "cli", "--product", "cloud", "--mutation=true", "--limit", "1", "--full", "--json"}
	if code := Run(t.Context(), args, &out, options); code != 0 {
		t.Fatalf("list code=%d output=%s", code, &out)
	}
	var first capabilitylist.Output
	decodeRecoveryHintOutput(t, out.Bytes(), &first)
	if len(first.Capabilities) != 1 || first.Capabilities[0].ID != "project.create" || !first.Capabilities[0].ExecutionEnabled || len(first.Help) == 0 || first.NextCommand == "" {
		t.Fatalf("unexpected first page: %s", &out)
	}
	t.Run("inspection", func(t *testing.T) {
		var out bytes.Buffer
		if code := Run(t.Context(), recoveryHintArguments(t, first.Help[0]), &out, options); code != 0 {
			t.Fatalf("hint=%q code=%d output=%s", first.Help[0], code, &out)
		}
		var inspected capabilityget.Output
		decodeRecoveryHintOutput(t, out.Bytes(), &inspected)
		if inspected.Capability.ID != first.Capabilities[0].ID || !inspected.Capability.ExecutionEnabled {
			t.Fatalf("inspection lost selected site consent: hint=%q output=%s", first.Help[0], &out)
		}
	})
	t.Run("continuation", func(t *testing.T) {
		var out bytes.Buffer
		if code := Run(t.Context(), recoveryHintArguments(t, first.NextCommand), &out, options); code != 0 {
			t.Fatalf("hint=%q code=%d output=%s", first.NextCommand, code, &out)
		}
		var next capabilitylist.Output
		if err := json.Unmarshal(out.Bytes(), &next); err != nil {
			t.Fatalf("continuation lost JSON output: %v output=%s", err, &out)
		}
		if next.Page.Total != first.Page.Total || next.Page.Limit != 1 || len(next.Capabilities) != 1 || next.Capabilities[0].ID != "project.delete" || !next.Capabilities[0].ExecutionEnabled || len(next.Capabilities[0].Selectors) == 0 {
			t.Fatalf("continuation lost context or full output: hint=%q output=%s", first.NextCommand, &out)
		}
	})
}

type recoveryHintPolicy struct {
	fixtureManagedPolicy
	status managedpolicy.Status
}

func (p recoveryHintPolicy) Status() managedpolicy.Status { return p.status }

func TestPolicyStatusExpansionRemainsAvailableThroughCLI(t *testing.T) {
	for _, state := range []string{managedpolicy.StateError, managedpolicy.StateActive} {
		t.Run(state, func(t *testing.T) {
			options := overviewOptions(t, t.TempDir())
			status := managedpolicy.Status{
				State: state, Path: "/fixed/system/policy.json", Protected: state == managedpolicy.StateActive,
				AllowedCapabilities: []string{"capability.list"},
				Checks:              []value.ManagedPolicyProtectionCheck{{Path: "/fixed/system/policy.json", Kind: "ownership", Passed: state == managedpolicy.StateActive}},
			}
			options.managedPolicy = recoveryHintPolicy{fixtureManagedPolicy: fixtureManagedPolicy{state: state, allowed: map[string]bool{"capability.list": true}}, status: status}
			selectedConfig := filepath.Join(t.TempDir(), "selected config's.yaml")
			if err := config.Save(selectedConfig, config.Config{Version: 1}); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if code := Run(t.Context(), []string{"--config", selectedConfig, "policy", "status", "--json"}, &out, options); code != 0 {
				t.Fatalf("status code=%d output=%s", code, &out)
			}
			var compact struct {
				Details string `json:"details"`
			}
			if err := json.Unmarshal(out.Bytes(), &compact); err != nil || compact.Details == "" {
				t.Fatalf("missing expansion: err=%v output=%s", err, &out)
			}
			args := recoveryHintArguments(t, compact.Details)
			if index := slices.Index(args, "--config"); index < 0 || index+1 >= len(args) || args[index+1] != selectedConfig {
				t.Fatalf("expansion lost configuration: %q", compact.Details)
			}
			out.Reset()
			if code := Run(t.Context(), args, &out, options); code != 0 {
				t.Fatalf("expansion blocked: hint=%q code=%d output=%s", compact.Details, code, &out)
			}
			var full struct {
				State      string                               `json:"state"`
				Details    string                               `json:"details"`
				AllowedIDs []string                             `json:"allowed_capability_ids"`
				Checks     []value.ManagedPolicyProtectionCheck `json:"protection_checks"`
			}
			if err := json.Unmarshal(out.Bytes(), &full); err != nil {
				t.Fatalf("expansion lost JSON output: %v output=%s", err, &out)
			}
			if full.State != state || full.Details != "" || !reflect.DeepEqual(full.AllowedIDs, status.AllowedCapabilities) || !reflect.DeepEqual(full.Checks, status.Checks) {
				t.Fatalf("expansion lost diagnostics: %s", &out)
			}
			out.Reset()
			if code := Run(t.Context(), []string{"last", "--full", "--json"}, &out, options); code == 0 || !strings.Contains(out.String(), "policy.denied") {
				t.Fatalf("last bypassed policy: code=%d output=%s", code, &out)
			}
		})
	}
}

func decodeRecoveryHintOutput(t *testing.T, data []byte, target any) {
	t.Helper()
	if !json.Valid(data) {
		decoded, err := toon.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		data, err = json.Marshal(decoded)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func recoveryHintArguments(t *testing.T, hint string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		command := exec.CommandContext(t.Context(), "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "function tadx { ConvertTo-Json -InputObject @($args | ForEach-Object { [string] $_ }) -Compress }; "+hint)
		data, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("parse hint: %v %s", err, data)
		}
		var args []string
		if err := json.Unmarshal(data, &args); err != nil {
			t.Fatal(err)
		}
		return args
	}
	command := exec.CommandContext(t.Context(), "sh", "-c", "tadx() { printf '%s\\0' \"$@\"; }; "+hint)
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("parse hint: %v %s", err, data)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}
