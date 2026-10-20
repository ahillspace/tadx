package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/commandhint"
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
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			options := diagnosticOptions(t, server)
			code := app.Run(context.Background(), args, &out, options)
			failure := decodeDiagnosticFailure(t, out.String())
			wantRecovery := commandhint.Command("--config", options.ConfigPath, "pulse", "definition", "list", "--datasource-id", "ds-1", "--full", "--environment", "test")
			if !strings.Contains(failure.CorrectiveAction, wantRecovery) {
				t.Errorf("mistargeted recovery: %s; want %s", failure.CorrectiveAction, wantRecovery)
			}
			for _, want := range []string{tt.want, "retryable: false", "rejected-create", strings.ReplaceAll(tt.detail, "diagnostic-session", "[REDACTED]")} {
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

// TestPulseCreateNamesEquivalentDefinitionBeforeSubmission covers Tableau's
// equivalence rule: a definition on the same datasource with the same measure,
// aggregation, time dimension, running total, temporality, and filters is
// rejected with a bare HTTP 409 whatever its name or dimensions.
func TestPulseCreateNamesEquivalentDefinitionBeforeSubmission(t *testing.T) {
	const equivalent = `"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"},"filters":[]},"is_running_total":false,"temporality":"TEMPORALITY_OVER_TIME"`
	for _, tt := range []struct {
		name, specification string
		rejected            bool
	}{
		{"equivalent", equivalent, true},
		{"aggregation", strings.Replace(equivalent, "AGGREGATION_SUM", "AGGREGATION_MAX", 1), false},
		{"time dimension", strings.Replace(equivalent, `"field":"Order Date"`, `"field":"Ship Date"`, 1), false},
		{"running total", strings.Replace(equivalent, `"is_running_total":false`, `"is_running_total":true`, 1), false},
		{"temporality", strings.Replace(equivalent, "TEMPORALITY_OVER_TIME", "TEMPORALITY_LATEST_POINT_IN_TIME", 1), false},
		{"filters", strings.Replace(equivalent, `"filters":[]`, `"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"}]}]`, 1), false},
		{"filters unknown", strings.Replace(equivalent, `,"filters":[]`, "", 1), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			creates := 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"definition-other-datasource","name":"Total Revenue"},"specification":{"datasource":{"id":"ds-2"},`+equivalent+`}},{"metadata":{"id":"definition-existing","name":"Total Revenue"},"specification":{"datasource":{"id":"ds-1"},`+tt.specification+`}}]}`)
						return
					}
					creates++
					w.WriteHeader(http.StatusConflict)
					_, _ = io.WriteString(w, `{"httpErrorCode":409,"message":"Conflict"}`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			args := []string{"pulse", "definition", "create", "--environment", "test", "--name", "Revenue", "--datasource-id", "ds-1", "--measure-field", "Revenue", "--aggregation", "SUM", "--date-field", "Order Date", "--dimension", "Region"}
			var out bytes.Buffer
			options := diagnosticOptions(t, server)
			code := app.Run(context.Background(), args, &out, options)
			failure := decodeDiagnosticFailure(t, out.String())
			if !tt.rejected {
				if creates != 1 || failure.ID != "pulse.definition.create.failed" {
					t.Fatalf("code=%d creates=%d output=%s", code, creates, out.String())
				}
				return
			}
			inspect := commandhint.Command("--config", options.ConfigPath, "pulse", "definition", "inspect", "--id", "definition-existing", "--environment", "test")
			if code == 0 || creates != 0 || failure.ID != "pulse.definition.create.equivalent" || !strings.Contains(failure.CorrectiveAction, inspect) || strings.Contains(out.String(), "definition-other-datasource") {
				t.Fatalf("code=%d creates=%d output=%s", code, creates, out.String())
			}
		})
	}
}
