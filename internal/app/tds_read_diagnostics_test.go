package app

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func TestTDSCheckoutPreviewRedactsExactLookupDiagnostics(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	const patName, patSecret, token = "fixture-pat-name", "fixture-pat-secret", "secret-token"
	t.Setenv("TADX_FIXTURE_NAME", patName)
	t.Setenv("TADX_FIXTURE_SECRET", patSecret)
	h.fixture.mu.Lock()
	h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet || r.URL.Path != "/api/3.30/sites/"+fixtureTDSSite+"/datasources/"+fixtureTDSOriginal {
			return false
		}
		if r.Header.Get("X-Tableau-Auth") != token {
			t.Error("exact lookup missing the fixture session")
		}
		w.Header().Set("X-Request-Id", "request-"+patSecret)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `<tsResponse><error code="403004"><summary>Denied %s</summary><detail>Reason %s %s</detail></error></tsResponse>`, patName, patSecret, token)
		return true
	}
	h.fixture.mu.Unlock()

	// Each invocation saves its own full receipt, including compact failures.
	for _, flag := range []string{"", "--full"} {
		args := []string{"content", "datasource", "checkout", "--id", fixtureTDSOriginal, "--preview"}
		if flag != "" {
			args = append(args, flag)
		}
		cmd := h.process(args...)
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := cmd.Run(); err == nil {
			t.Fatal("checkout preview succeeded after its exact lookup was denied")
		}
		last := h.command(true, "last")
		for label, data := range map[string]string{"command": out.String(), "stderr": stderr.String(), "last": last} {
			for _, secret := range []string{patName, patSecret, token} {
				if strings.Contains(data, secret) {
					t.Errorf("%s %s exposed a fixture credential", flag, label)
				}
			}
		}
		for _, data := range []string{out.String(), last} {
			for _, want := range []string{"403004", "request-[REDACTED]"} {
				if !strings.Contains(data, want) {
					t.Errorf("%s missing diagnostic %q", flag, want)
				}
			}
		}
		requireTDSJSON(t, last, "Denied [REDACTED]", "Reason [REDACTED] [REDACTED]")
	}
}

const retiredTDSFixtureToken = "retired-fixture-token"

// Issue a short-lived token and reject the source GET once. The fixture's
// ordinary sign-in issues the replacement for the remaining requests.
func renewTDSLookup(h *tdsWorkflowCLI, after func(http.ResponseWriter, *http.Request) bool) {
	first := true
	h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/3.30/auth/signin" && first {
			first = false
			fmt.Fprintf(w, `{"credentials":{"token":%q,"site":{"id":%q},"user":{"id":"user-luid"}}}`, retiredTDSFixtureToken, fixtureTDSSite)
			return true
		}
		if r.Header.Get("X-Tableau-Auth") == retiredTDSFixtureToken {
			if r.Method != http.MethodGet || r.URL.Path != "/api/3.30/sites/"+fixtureTDSSite+"/datasources/"+fixtureTDSOriginal {
				h.t.Errorf("unexpected request before renewal: %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `<tsResponse><error code="401002"><summary>Session expired</summary></error></tsResponse>`)
			return true
		}
		return after(w, r)
	}
}

func TestTDSCheckoutRejectsRetiredTokenInSuccessfulLookup(t *testing.T) {
	for mode, preview := range map[string]bool{"preview": true, "execute": false} {
		t.Run(mode, func(t *testing.T) {
			h := newTDSWorkflowCLI(t)
			h.options = withSiteMutationConsent(t, h.options, true)
			renewTDSLookup(h, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/api/3.30/sites/"+fixtureTDSSite+"/datasources/"+fixtureTDSOriginal {
					return false
				}
				if r.Header.Get("X-Tableau-Auth") != "secret-token" {
					t.Error("lookup used the wrong replacement token")
				}
				fmt.Fprintf(w, `<tsResponse><datasource id="%s" name="Source %s"><project id="%s" name="Test"/></datasource></tsResponse>`, fixtureTDSOriginal, retiredTDSFixtureToken, fixtureTDSProject)
				return true
			})
			args := []string{"checkout", "--id", fixtureTDSOriginal}
			if preview {
				args = append(args, "--preview")
			}
			out := h.ds(false, args...)
			last := h.command(true, "last")
			for _, data := range []string{out, last} {
				if strings.Contains(data, retiredTDSFixtureToken) || !strings.Contains(data, "protected credential") {
					t.Error("checkout did not safely reject the protected source")
				}
			}
			journal, err := operationrun.DefaultTDSJournal()
			if err != nil {
				t.Fatal(err)
			}
			attempts, _, err := journal.Attempts(t.Context(), value.TDSScope{Server: h.server.URL, SiteLUID: fixtureTDSSite, CallerLUID: "user-luid"}, "", "", "")
			if err != nil || len(attempts) != 0 || h.fixture.writeCount() != 0 {
				t.Errorf("attempts=%d writes=%d err=%v", len(attempts), h.fixture.writeCount(), err)
			}
		})
	}
}

func TestTDSAuditRedactsRetiredTokenInIndexedField(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	renewTDSLookup(h, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/metadata/graphql" {
			return false
		}
		if r.Header.Get("X-Tableau-Auth") != "secret-token" {
			t.Error("indexed field query used the wrong replacement token")
		}
		fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{
			"id":"source","luid":%q,"name":"Indexed source",
			"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{
				"id":"indexed-field","name":"Indexed field","fullyQualifiedName":"[Indexed field]",
				"description":"Historical value %s","descriptionInherited":[],
				"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},
				"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}
			}]}
		}]}}}`, fixtureTDSOriginal, retiredTDSFixtureToken)
		return true
	})
	out := h.ds(true, "checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", "audit.md")
	audit, err := os.ReadFile(filepath.Join(h.ws, "audit.md"))
	if err != nil {
		t.Fatal(err)
	}
	last := h.command(true, "last")
	if strings.Contains(string(audit), retiredTDSFixtureToken) || !strings.Contains(string(audit), "Historical value [REDACTED]") ||
		strings.Contains(out, retiredTDSFixtureToken) || strings.Contains(last, retiredTDSFixtureToken) {
		t.Fatal("indexed field observation was not safely exported")
	}
	if h.fixture.writeCount() != 1 {
		t.Fatalf("expected one checkout, got %d writes", h.fixture.writeCount())
	}
}
