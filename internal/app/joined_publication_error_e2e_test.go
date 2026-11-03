package app_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestFailedFlowPublicationRetainsReceiptFailure(t *testing.T) {
	var writes atomic.Int32
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
			fmt.Fprint(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations" topLevelProject="true"/></projects></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/flows"):
			fmt.Fprint(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><flows/></tsResponse>`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/flows"):
			writes.Add(1)
			w.Header().Set("X-Tableau-Request-Id", "uncertain-flow-request")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `<tsResponse><error code="500000"><summary>Service unavailable</summary></error></tsResponse>`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	options.JobDirectory = filepath.Join(t.TempDir(), "blocked-jobs")
	if err := os.WriteFile(options.JobDirectory, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	flow := filepath.Join(t.TempDir(), "Daily.tfl")
	if err := os.WriteFile(flow, []byte(`{"flow":"daily"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	options = withSiteMutationConsent(t, options, true)
	var out strings.Builder
	code := app.Run(t.Context(), []string{"content", "flow", "publish", "--file", flow, "--environment", "test", "--project-id", "project-1", "--full", "--json"}, &out, options)
	if code == 0 || writes.Load() != 1 {
		t.Fatalf("code=%d writes=%d output=%s", code, writes.Load(), out.String())
	}
	assertErrors := func(label, text string) {
		t.Helper()
		for _, want := range []string{`"flow.publish.failed"`, `"flow.publish.receipt"`, `"phase":"persistence"`, `"outcome":"unknown"`, "uncertain-flow-request"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %s: %s", label, want, text)
			}
		}
	}
	assertErrors("live result", out.String())
	out.Reset()
	if code := app.Run(t.Context(), []string{"last", "--full", "--json"}, &out, options); code != 0 {
		t.Fatalf("last code=%d output=%s", code, out.String())
	}
	assertErrors("saved result", out.String())
}
