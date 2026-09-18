package job

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
)

type session struct{}

func (session) String() string          { return "test job session" }
func (session) Authorize(*http.Request) {}
func (session) SiteLUID() string        { return "site" }
func (session) UserLUID() string        { return "user" }

func TestInspectAuthoritativeJobStates(t *testing.T) {
	for _, test := range []struct {
		name, attributes, child, want string
		bad                           bool
	}{
		{"pending publish", `id="job" type="PublishWorkbook" progress="0" finishCode="1"`, "", "pending", false},
		{"complete publish", `id="job" type="PublishDatasource" progress="100" finishCode="0"`, `<datasource id="ds"/>`, "succeeded", false},
		{"failed publish", `id="job" type="PublishWorkbook" progress="100" finishCode="1"`, "", "failed", false},
		{"cancelled", `id="job" type="RefreshExtract" progress="100" finishCode="2"`, "", "cancelled", false},
		{"active extract", `id="job" type="RefreshExtract" progress="25" finishCode="0"`, "", "running", false},
		{"bridge assigned", `id="job" type="BridgeRefreshExtract" progress="100" finishCode="0"`, "", "running", false},
		{"bridge complete", `id="job" type="BridgeRefreshExtract" progress="100" finishCode="3"`, "", "succeeded", false},
		{"mismatch", `id="different" type="PublishWorkbook" progress="100" finishCode="0"`, "", "", true},
		{"missing state", `id="job" type="PublishWorkbook"`, "", "", true},
		{"invalid state", `id="job" type="PublishWorkbook" progress="110" finishCode="0"`, "", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/3.29/sites/site/jobs/job" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("X-Tableau-Request-Id", "request")
				fmt.Fprintf(w, "<tsResponse><job %s>%s</job></tsResponse>", test.attributes, test.child)
			}))
			defer server.Close()
			got, err := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL).Inspect(t.Context(), "job")
			if (err != nil) != test.bad || !test.bad && (got.Status != test.want || got.CheckedAt.IsZero() || got.RequestID != "request") {
				t.Fatalf("got=%+v err=%v", got, err)
			}
			if test.child != "" && got.ResourceID != "ds" {
				t.Fatalf("destination identity lost: %+v", got)
			}
		})
	}
}

func TestCancelIsExactlyOneRequestAndDoesNotInventTerminalState(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPut || r.URL.Path != "/api/3.29/sites/site/jobs/job" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("X-Tableau-Request-Id", "cancel-request")
	}))
	defer server.Close()
	requestID, err := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL).Cancel(t.Context(), "job")
	if err != nil || requestID != "cancel-request" || calls != 1 {
		t.Fatalf("id=%s calls=%d err=%v", requestID, calls, err)
	}
}
