package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	policyops "github.com/ahillspace/tadx/actions/policy"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/managedpolicy"
)

func TestPolicyWorkflowValidationEnvelopes(t *testing.T) {
	for _, test := range []struct {
		name, id string
		args     []string
		output   bool
	}{
		{"samples blank", "policy.samples.usage", []string{"samples", "--output", ""}, false},
		{"samples whitespace", "policy.samples.usage", []string{"samples", "--output", "  "}, false},
		{"validate blank", "policy.validate.usage", []string{"validate", ""}, false},
		{"validate whitespace", "policy.validate.usage", []string{"validate", "  "}, false},
		{"install unknown", "policy.install.usage", []string{"install", "--template", "owner"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			args := append([]string{"policy"}, test.args...)
			args = append(args, "--json")
			code := Run(t.Context(), args, &out, overviewOptions(t, t.TempDir()))
			var result struct {
				Output json.RawMessage `json:"output"`
				Error  struct {
					ID      string `json:"id"`
					Phase   string `json:"phase"`
					Outcome string `json:"outcome"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("output=%s error=%v", &out, err)
			}
			if code == 0 || result.Error.ID != test.id || result.Error.Phase != "validation" || result.Error.Outcome != "not_attempted" || (len(result.Output) > 0) != test.output {
				t.Fatalf("code=%d output=%s", code, &out)
			}
		})
	}
}

func TestPolicyInstallNormalizesTemplateBeforeNativePathValidation(t *testing.T) {
	for _, template := range []string{"", " ", "admin", " admin ", "read-only", "read-write-no-admin", "superuser"} {
		t.Run(template, func(t *testing.T) {
			var out bytes.Buffer
			code := Run(t.Context(), []string{"policy", "install", "--output", "relative-policy-fixture", "--template", template, "--json"}, &out, overviewOptions(t, t.TempDir()))
			var result struct {
				Output policyops.InstallOutput `json:"output"`
				Error  struct {
					ID string `json:"id"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			want := template
			if template == "" || template == " " || template == "admin" || template == " admin " {
				want = "superuser"
			}
			if code == 0 || result.Error.ID != "policy.install.failed" || result.Output.Template != want || result.Output.Phase != "validation" || result.Output.PolicyWritten || result.Output.ProtectionChanged || result.Output.Active || result.Output.LocatorPublished {
				t.Fatalf("code=%d output=%s", code, &out)
			}
		})
	}
}

func TestPolicyInstallReceiptWireShapesRemainDistinct(t *testing.T) {
	for _, test := range []struct {
		value any
		want  string
	}{
		{managedpolicy.InstallResult{}, `{"path":"","template":"","policy_written":false,"locator_published":false,"active":false,"protection_changed":false,"phase":""}`},
		{policyops.InstallOutput{}, `{"path":"","template":"","protection_changed":false,"policy_written":false,"locator_published":false,"active":false,"phase":""}`},
	} {
		data, err := json.Marshal(test.value)
		if err != nil || string(data) != test.want {
			t.Fatalf("receipt=%s want=%s error=%v", data, test.want, err)
		}
	}
}

func TestPolicyWorkflowPreservesExactCandidatePaths(t *testing.T) {
	t.Chdir(t.TempDir())
	options := overviewOptions(t, t.TempDir())
	var out bytes.Buffer
	if code := Run(t.Context(), []string{"policy", "samples", "--output", " samples", "--json"}, &out, options); code != 0 {
		t.Fatalf("samples code=%d output=%s", code, &out)
	}
	candidate := filepath.Join(" samples", "read-only.json")
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("exact output directory was changed: %v", err)
	}
	out.Reset()
	if code := Run(t.Context(), []string{"policy", "validate", candidate, "--json"}, &out, options); code != 0 {
		t.Fatalf("validate code=%d output=%s", code, &out)
	}
	var result policyops.ValidationOutput
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Candidate != filepath.ToSlash(candidate) {
		t.Fatalf("candidate path changed: %s error=%v", &out, err)
	}
}

// WriteSamples checks cancellation before each file after directory/preflight work.
type cancelPolicySamplesAfterFirst struct {
	context.Context
	checks int
}

func (c *cancelPolicySamplesAfterFirst) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.Canceled
	}
	return nil
}

func TestPolicySamplesRetainsConfirmedFilesOnCancellation(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "samples")
	runtime := &runtimeDependencies{managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateUnmanaged}}
	out, err := runtime.WriteSamples(&cancelPolicySamplesAfterFirst{Context: t.Context()}, directory)
	structured, ok := errors.AsType[*errs.Error](err)
	want := filepath.ToSlash(filepath.Join(directory, "read-only.json"))
	if !ok || !errors.Is(err, context.Canceled) || structured.ID != "policy.samples.write" || structured.Outcome != errs.OutcomeUnknown || out.Status != "partial" || !slices.Equal(out.Files, []string{want}) || !slices.Equal(structured.Completed, out.Files) {
		t.Fatalf("output=%+v error=%v", out, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "read-write-no-admin.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected second candidate: %v", err)
	}
}
