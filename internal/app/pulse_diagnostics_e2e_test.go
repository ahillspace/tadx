package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestPulseCreateRejectionDiagnosticsThroughCLI(t *testing.T) {
	for _, tt := range []struct {
		name, detail, want string
		status             int
	}{
		{name: "bad request", status: 400, detail: "Bad Request", want: "--preview --full"},
		{name: "validation detail", status: 400, detail: "Unsupported configuration diagnostic-session", want: "--preview --full"},
		{name: "unspecified conflict", status: 409, detail: "Conflict", want: "equivalent definition"},
		{name: "reported duplicate", status: 409, detail: "Metric specification already exists", want: "equivalent definition"},
		{name: "uncertain response", status: 503, detail: "Service unavailable", want: "Reconcile"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			creates := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch r.URL.Path {
				case "/api/3.29/sites/site-1/datasources/ds-1":
					_, _ = io.WriteString(w, `<tsResponse><datasource id="ds-1" name="Orders"><project id="project-1" name="Test"/></datasource></tsResponse>`)
				case "/api/v1/vizql-data-service/read-metadata":
					_, _ = io.WriteString(w, `{"data":[{"fieldName":"Revenue","dataType":"REAL","fieldRole":"MEASURE"},{"fieldName":"Order Date","dataType":"DATE","fieldRole":"DIMENSION"},{"fieldName":"Region","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
				case "/api/-/pulse/definitions":
					if r.Method == http.MethodGet {
						_, _ = io.WriteString(w, `{"definitions":[]}`)
						return
					}
					creates++
					w.Header().Set("X-Tableau-Request-Id", "rejected-create")
					w.WriteHeader(tt.status)
					_, _ = fmt.Fprintf(w, `{"error":{"code":"%d","summary":"Request rejected","detail":%q}}`, tt.status, tt.detail)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			args := []string{"pulse", "definition", "create", "--environment", "test", "--name", "Revenue", "--datasource-id", "ds-1", "--measure-field", "Revenue", "--aggregation", "SUM", "--date-field", "Order Date", "--dimension", "Region", "--full"}
			var out bytes.Buffer
			code := app.Run(context.Background(), args, &out, diagnosticOptions(t, server))
			for _, want := range []string{tt.want, "tadx pulse definition list", "retryable: false", "rejected-create", strings.ReplaceAll(tt.detail, "diagnostic-session", "[REDACTED]")} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q: %s", want, out.String())
				}
			}
			if code == 0 || creates != 1 || strings.Contains(out.String(), "diagnostic-session") {
				t.Errorf("code=%d creates=%d output=%s", code, creates, out.String())
			}
		})
	}
}
