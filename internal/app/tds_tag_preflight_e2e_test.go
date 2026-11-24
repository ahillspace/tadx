package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	ds "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

func TestTDSTagPreflightReadFailure(t *testing.T) {
	const label = "Quarterly sales"
	for _, tc := range []struct {
		verb       string
		failAt     int32
		statusCode int
	}{
		{"remove-tag", 1, http.StatusForbidden},
		{"add-tag", 2, http.StatusServiceUnavailable},
	} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/full=%t", tc.verb, full), func(t *testing.T) {
				h := newTDSWorkflowCLI(t)
				h.options = withSiteMutationConsent(t, h.options, true)
				var reads atomic.Int32
				h.fixture.mu.Lock()
				h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
					if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/datasources/"+fixtureTDSOriginal) || reads.Add(1) != tc.failAt {
						return false
					}
					w.Header().Set("X-Request-Id", "tag-read-refused")
					w.WriteHeader(tc.statusCode)
					fmt.Fprintf(w, `<tsResponse><error code="%d000"><summary>Tag read unavailable</summary></error></tsResponse>`, tc.statusCode)
					return true
				}
				h.fixture.mu.Unlock()

				args := []string{"content", "datasource", tc.verb, "--id", fixtureTDSOriginal, "--tag", label, "--json"}
				if tc.failAt == 1 {
					args = append(args, "--preview")
				}
				if full {
					args = append(args, "--full")
				}
				body, runErr := h.process(args...).Output()
				var got struct {
					Error  errs.Payload
					Output ds.TagOutput
				}
				if runErr == nil || json.Unmarshal(body, &got) != nil || reads.Load() != tc.failAt || h.fixture.writeCount() != 0 {
					t.Fatalf("expected read refusal without submission, got %s (%v)", body, runErr)
				}
				e, o := got.Error, got.Output
				preview := commandhint.Environment("dev", "--config", h.options.ConfigPath, "content", "datasource", tc.verb, "--id", fixtureTDSOriginal, "--tag", label, "--preview")
				if e.Operation != "datasource."+tc.verb || e.Resource != fixtureTDSOriginal || e.Environment != "dev" || e.Phase != errs.PhaseSetup || e.Outcome != errs.OutcomeNotAttempted || e.UpstreamStatus != tc.statusCode || e.TableauRequestID != "tag-read-refused" || e.Retryable == nil || *e.Retryable || e.CorrectiveAction != preview {
					t.Errorf("missing tag failure context: %s", body)
				}
				if o.DatasourceLUID != fixtureTDSOriginal || o.Environment != "dev" || o.Tag != label || o.Status != "not_attempted" || o.Desired != (tc.verb == "add-tag") || o.Observed != nil {
					t.Errorf("incorrect tag mutation state: %s", body)
				}
				if tc.failAt == 1 && o.PresentBefore != nil || tc.failAt == 2 && (o.PresentBefore == nil || *o.PresentBefore) {
					t.Errorf("incorrect membership before submission: %s", body)
				}
			})
		}
	}
}
