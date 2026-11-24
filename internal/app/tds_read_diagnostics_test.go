package app

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"
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
