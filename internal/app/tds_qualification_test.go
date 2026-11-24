package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ds "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/config"
)

func TestTDSProductionQualificationStopsBeforeProviderAccess(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	// Even an explicitly permitted target cannot satisfy the evidence gate.
	options := withSiteMutationConsent(t, h.options, true)
	// overviewOptions installs a transport and credential store that fail the
	// test on access. These production invocations must succeed or fail offline.
	plan := fmt.Sprintf("target:\n  server: %s\n  site_luid: %s\n  work_copy_luid: %s\nchanges:\n  - op: set-field-description\n    field: Sales\n    description: Gross sales\n", h.server.URL, fixtureTDSSite, fixtureTDSCopy)
	if err := os.WriteFile(filepath.Join(h.ws, "intent.yaml"), []byte(plan), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	validate := []string{"content", "datasource", "validate-changes", "--workspace", "work", "--from-changes", "intent.yaml", "--json"}
	if code := Run(t.Context(), validate, &out, options); code != 0 || !json.Valid(out.Bytes()) {
		t.Fatalf("offline validation %d: %s", code, &out)
	}
	requireTDSJSON(t, out.String(), `"status":"valid"`, `"operations":["set-field-description"]`, fixtureTDSCopy)
	if err := os.WriteFile(filepath.Join(h.ws, "intent.yaml"), []byte(strings.Replace(plan, "description: Gross sales", "tag: unsupported", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := Run(t.Context(), validate, &out, options); code == 0 || !json.Valid(out.Bytes()) {
		t.Fatalf("invalid plan returned %d: %s", code, &out)
	}

	for _, tc := range []struct {
		op      string
		args    []string
		preview bool
	}{
		{"checkout", []string{"--id", fixtureTDSOriginal}, true},
		{"elements", []string{"--id", fixtureTDSOriginal}, false},
		{"work-copy", []string{"--work-copy", fixtureTDSCopy}, false},
		{"set-field-description", []string{"--work-copy", fixtureTDSCopy, "--field", "Sales", "--description", "Sales total"}, true},
		{"remove-field-description", []string{"--work-copy", fixtureTDSCopy, "--field", "Sales"}, true},
		{"set-table-description", []string{"--work-copy", fixtureTDSCopy, "--table", "Sales", "--description", "Sales orders"}, true},
		{"remove-table-description", []string{"--work-copy", fixtureTDSCopy, "--table", "Sales"}, true},
		{"apply-changes", []string{"--workspace", "work", "--from-changes", "intent.yaml"}, true},
		{"save.as-new", []string{"--work-copy", fixtureTDSCopy, "--name", "New"}, true},
		{"save.overwrite", []string{"--work-copy", fixtureTDSCopy}, true},
		{"add-tag", []string{"--id", fixtureTDSOriginal, "--tag", "Quarterly"}, true},
		{"remove-tag", []string{"--id", fixtureTDSOriginal, "--tag", "Quarterly"}, true},
	} {
		t.Run(tc.op, func(t *testing.T) {
			args := append([]string{"content", "datasource"}, strings.Split(tc.op, ".")...)
			args = append(args, tc.args...)
			args = append(args, "--json")
			for _, preview := range []bool{false, true} {
				if preview && !tc.preview {
					continue
				}
				var stdout bytes.Buffer
				cmd := args
				if preview {
					cmd = append(cmd, "--preview")
				}
				exit := Run(t.Context(), cmd, &stdout, options)
				var got struct {
					Error struct{ ID, Operation, Outcome string }
				}
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || exit == 0 || got.Error.ID != "datasource.evidence_required" || got.Error.Operation != "datasource."+tc.op || got.Error.Outcome != "not_attempted" {
					t.Fatalf("exit %d, stdout %s", exit, &stdout)
				}
			}
		})
	}
}

type tdsAfterRead struct {
	http.RoundTripper
	after func(*http.Request)
}

func (h tdsAfterRead) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := h.RoundTripper.RoundTrip(r)
	if err == nil && r.Method == http.MethodGet {
		h.after(r)
	}
	return response, err
}

func TestTDSRevokedConsentPreservesVerifiedPrefixAtDispatch(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	h.ds(true, "checkout", "--id", fixtureTDSOriginal)
	plan := fmt.Sprintf("target:\n  server: %s\n  site_luid: %s\n  work_copy_luid: %s\nchanges:\n  - op: set-field-description\n    field: Sales\n    description: Gross sales\n  - op: set-table-description\n    table: Orders\n    description: Line items\n", h.server.URL, fixtureTDSSite, fixtureTDSCopy)
	if err := os.WriteFile(filepath.Join(h.ws, "two.yaml"), []byte(plan), 0600); err != nil {
		t.Fatal(err)
	}
	tablesRead := 0
	opts := h.options
	opts.HTTPClient = &http.Client{Transport: tdsAfterRead{h.server.Client().Transport, func(r *http.Request) {
		if r.URL.Query().Get("elementType") != "TableDescription" {
			return
		}
		tablesRead++
		if tablesRead != 2 {
			return
		}
		// The first item has been verified. Revoke consent in the same config
		// file immediately after the second item's final pre-write observation.
		cfg, err := config.Load(opts.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.SetMutationSetting(cfg.Environments["dev"], false); err != nil {
			t.Fatal(err)
		}
		if err := config.Save(opts.ConfigPath, cfg); err != nil {
			t.Fatal(err)
		}
	}}}
	var out bytes.Buffer
	args := []string{"content", "datasource", "apply-changes", "--workspace", "work", "--from-changes", "two.yaml", "--json", "--full"}
	exit := run(t.Context(), args, &out, opts, qualifiedTDSDefinitions())
	if exit == 0 || !json.Valid(out.Bytes()) || tablesRead != 2 {
		t.Fatalf("exit=%d table reads=%d stdout=%s", exit, tablesRead, &out)
	}
	var result struct {
		Output ds.EditOutput
		Error  struct {
			Outcome   string
			Completed []string
			Failed    string
		}
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Error.Outcome != "not_attempted" || len(result.Output.Items) != 2 {
		t.Fatalf("incomplete command evidence: %s", &out)
	}
	if len(result.Error.Completed) != 1 || result.Error.Completed[0] != "1: set-field-description" || result.Error.Failed != "2: set-table-description" {
		t.Fatalf("structured failure lost the completed prefix: %s", &out)
	}
	first, second := result.Output.Items[0], result.Output.Items[1]
	if first.Op != "set-field-description" || first.Status != "verified" || first.Observed == nil || first.Observed.Value != "Gross sales" || second.Op != "set-table-description" || second.Submission != "not_attempted" || second.Status != "unattempted" {
		t.Fatalf("incorrect ordered receipts: %+v", result.Output.Items)
	}
	h.fixture.mu.Lock()
	defer h.fixture.mu.Unlock()
	if len(h.fixture.writes) != 2 || h.fixture.writes[0] != "checkout" || h.fixture.writes[1] != "POST FieldDescription/Sales" {
		t.Fatalf("remote writes: %v", h.fixture.writes)
	}
}
