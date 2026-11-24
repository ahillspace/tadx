package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ds "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/value"
)

func TestTDSEligibilityFollowsObservedOperation(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	read := h.ds(true, "elements", "--id", fixtureTDSOriginal)
	var inspection ds.CopyInspection
	if err := json.Unmarshal([]byte(read), &inspection); err != nil || inspection.Checkout != nil || len(inspection.Observations) != 3 {
		t.Fatalf("published inspection %s", read)
	}
	for _, row := range inspection.Observations {
		e := row.Eligibility
		if e.Operation != "datasource.tds.read.datasource" || e.Status != "supported" || e.Reason != "read_confirmed" || e.Entitlement != "unknown" || e.OpenCopyQuota != "" || e.SourceType != "" {
			t.Errorf("read evidence %+v", e)
		}
	}
	checkout := []string{"checkout", "--id", fixtureTDSOriginal}
	preview := h.ds(true, append(checkout, "--preview")...)
	var planned ds.LifecycleOutput
	if err := json.Unmarshal([]byte(preview), &planned); err != nil || planned.SourceLUID != fixtureTDSOriginal || planned.WorkCopyLUID != "" || h.fixture.writeCount() != 0 {
		t.Fatalf("checkout preview %s", preview)
	}
	unknown := value.TDSEligibility{Operation: "datasource.checkout", Status: "unknown", Reason: "operation_not_observed", RESTVersion: "3.30", RESTStatus: "supported", SourceType: "unknown", Permission: "unknown", Entitlement: "unknown", OpenCopyQuota: "unknown"}
	if planned.Eligibility != unknown {
		t.Fatalf("unsubmitted checkout evidence %+v", planned.Eligibility)
	}
	h.options = withSiteMutationConsent(t, h.options, true)
	check := h.ds(true, append(checkout, "--workspace", "work", "--audit-file", "evidence.md")...)
	var confirmed ds.LifecycleOutput
	if err := json.Unmarshal([]byte(check), &confirmed); err != nil || confirmed.Checkout == nil || confirmed.SourceLUID != fixtureTDSOriginal || confirmed.WorkCopyLUID != fixtureTDSCopy || confirmed.Eligibility != confirmed.Checkout.Eligibility || confirmed.AttemptID == "" {
		t.Fatalf("checkout %s", check)
	}
	e := confirmed.Eligibility
	if e.Operation != "datasource.checkout" || e.Status != "supported" || e.RESTVersion != "3.30" || e.Reason != "operation_confirmed" || e.Entitlement != "supported" || e.Permission != "supported" || e.SourceType != "supported" || e.OpenCopyQuota != "supported" {
		t.Fatalf("acknowledged checkout evidence %+v", e)
	}
	attempt := h.ds(true, "work-copy", "--attempt", confirmed.AttemptID)
	requireTDSJSON(t, attempt, confirmed.AttemptID, fixtureTDSOriginal, fixtureTDSCopy, `"reason":"operation_confirmed"`)
	audit, err := os.ReadFile(filepath.Join(h.ws, "evidence.md"))
	if err != nil || !strings.Contains(string(audit), "read_confirmed") || !strings.Contains(string(audit), "operation_confirmed") {
		t.Fatalf("audit evidence %v, %s", err, audit)
	}
	edit := []string{"set-field-description", "--work-copy", fixtureTDSCopy, "--field", "Sales", "--description", "Gross sales"}
	plan := h.ds(true, append(edit, "--preview")...)
	requireTDSJSON(t, plan, `"operation":"datasource.tds.add"`, `"reason":"operation_not_observed"`, `"permission":"unknown"`)
	if h.fixture.writeCount() != 1 {
		t.Fatal("edit preview wrote to Tableau")
	}
	actual := h.ds(true, edit...)
	requireTDSJSON(t, actual, `"operation":"datasource.tds.add"`, `"reason":"operation_confirmed"`, `"status":"verified"`)
	review := fmt.Sprintf("target:\n  server: %s\n  site_luid: %s\n  datasource_luid: %s\n  work_copy_luid: %s\nchanges:\n  - op: set-field-description\n    field: Sales\n    description: Gross sales\n", h.server.URL, fixtureTDSSite, fixtureTDSOriginal, fixtureTDSCopy)
	if err := os.WriteFile(filepath.Join(h.ws, "review.yaml"), []byte(review), 0600); err != nil {
		t.Fatal(err)
	}
	got := h.ds(true, "work-copy", "--workspace", "work", "--from-changes", "review.yaml")
	requireTDSJSON(t, got, `"operation":"datasource.tds.read.workcopy"`, `"reason":"read_confirmed"`, `"status":"matched"`)
}

// The fixture receives normal classic requests. Only its advertised version
// and the single checkout response vary, with exact counts for dispatched TDS writes.
// The diagnostic strings below are illustrative condition names from the reference.
// Actual provider wording outside those explicit conditions remains unclassified.
type tdsEligibilityTransport struct {
	http.RoundTripper
	version, code, condition string
	status, submissions      int
}

func (tr *tdsEligibilityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response := func(code int, body string) *http.Response {
		return &http.Response{StatusCode: code, Status: fmt.Sprintf("%d %s", code, http.StatusText(code)), Request: r, Header: http.Header{"X-Request-Id": {"eligibility-refusal"}}, Body: io.NopCloser(strings.NewReader(body))}
	}
	if r.URL.Path == "/api/2.4/serverinfo" {
		return response(200, "<tsResponse><serverInfo><restApiVersion>"+tr.version+"</restApiVersion></serverInfo></tsResponse>"), nil
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+fixtureTDSOriginal+"/workcopy") {
		tr.submissions++
		return response(tr.status, fmt.Sprintf(`{"error":{"code":%q,"summary":%q}}`, tr.code, tr.condition)), nil
	}
	r = r.Clone(r.Context())
	r.URL.Path = strings.Replace(r.URL.Path, "/api/"+tr.version+"/", "/api/3.30/", 1)
	return tr.RoundTripper.RoundTrip(r)
}

func TestTDSCheckoutEligibilityRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, version, code, condition                                     string
		status                                                             int
		expectedStatus, reason, permission, entitlement, sourceType, quota string
	}{
		{"older REST", "3.29", "", "", 0, "unsupported", "rest_version_too_low", "unknown", "unknown", "unknown", "unknown"},
		{"composable", "3.30", "405003", "Method not allowed", 405, "unsupported", "composable_datasource", "unknown", "unknown", "unsupported", "unknown"},
		{"SDM", "3.30", "405004", "Method not allowed", 405, "unsupported", "semantic_data_model_datasource", "unknown", "unknown", "unsupported", "unknown"},
		{"permission", "3.30", "", "Permission denied", 403, "unsupported", "permission_denied", "unsupported", "unknown", "unknown", "unknown"},
		{"ambiguous forbidden", "3.30", "403004", "Forbidden", 403, "unknown", "forbidden_cause_unknown", "unknown", "unknown", "unknown", "unknown"},
		{"unavailable entitlement", "3.30", "403004", "Unable to resolve tenant", 403, "unknown", "entitlement_check_unavailable", "unknown", "unknown", "unknown", "unknown"},
		{"missing entitlement", "3.30", "403004", "Data Management Bundle not licensed", 403, "unsupported", "missing_entitlement", "unknown", "unsupported", "unknown", "unknown"},
		{"rate limit", "3.30", "429000", "Rate limit exceeded", 429, "unknown", "rate_limited", "unknown", "unknown", "unknown", "unknown"},
		{"open copy limit", "3.30", "", "Work copy limit exceeded", 429, "unsupported", "open_work_copy_limit", "unknown", "unknown", "unknown", "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newTDSWorkflowCLI(t)
			opts := withSiteMutationConsent(t, h.options, true)
			tr := &tdsEligibilityTransport{RoundTripper: h.server.Client().Transport, version: tc.version, code: tc.code, condition: tc.condition, status: tc.status}
			opts.HTTPClient = &http.Client{Transport: tr}
			args := []string{"content", "datasource", "checkout", "--id", fixtureTDSOriginal, "--json", "--full"}
			if tc.status == 0 {
				args = append(args, "--preview")
			}
			var out bytes.Buffer
			exit := run(t.Context(), args, &out, opts, qualifiedTDSDefinitions())
			var result struct{ Output ds.LifecycleOutput }
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || exit == 0 {
				t.Fatalf("checkout exit=%d stdout=%s", exit, &out)
			}
			o, e := result.Output, result.Output.Eligibility
			if e.Operation != "datasource.checkout" || e.RESTVersion != tc.version || e.Status != tc.expectedStatus || e.Reason != tc.reason || e.Permission != tc.permission || e.Entitlement != tc.entitlement || e.SourceType != tc.sourceType || e.OpenCopyQuota != tc.quota || o.SourceLUID != fixtureTDSOriginal || o.WorkCopyLUID != "" {
				t.Fatalf("unexpected evidence %s", &out)
			}
			if tc.status == 0 {
				if tr.submissions != 0 || o.AttemptID != "" || o.Mode != "preview" || o.Status != "not_attempted" || e.RESTStatus != "unsupported" {
					t.Fatalf("incompatible checkout preview %s", &out)
				}
			} else {
				if tr.submissions != 1 || o.AttemptID == "" || o.RequestID != "eligibility-refusal" || o.Status != "not_applied" || e.RESTStatus != "supported" {
					t.Fatalf("rejection receipt %s", &out)
				}
				retained := h.ds(true, "work-copy", "--attempt", o.AttemptID)
				requireTDSJSON(t, retained, o.RequestID, tc.reason, fixtureTDSOriginal)
			}
		})
	}
}
