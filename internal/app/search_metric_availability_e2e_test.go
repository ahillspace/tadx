package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestCloudMetricAvailabilityAdviceMatchesDeployment(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var nativeCalls atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if diagnosticSignIn(writer, request) {
					return
				}
				if request.URL.Path != "/api/-/search" || request.URL.Query().Get("filter") != "type:eq:insightmetric" {
					t.Errorf("unexpected fallback or request: %s %s", request.Method, request.URL.String())
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				nativeCalls.Add(1)
				writer.Header().Set("X-Tableau-Request-Id", "metric-endpoint-unavailable")
				writer.WriteHeader(status)
				_, _ = io.WriteString(writer, `{"message":"search unavailable"}`)
			}))
			t.Cleanup(server.Close)
			args := []string{"search", "Revenue", "--type", "metric", "--environment", "test", "--json"}
			var output strings.Builder
			code := app.Run(t.Context(), args, &output, cloudPulseSearchOptions(t, server))
			var cloudDocument struct {
				Error struct {
					ID               string `json:"id"`
					CorrectiveAction string `json:"corrective_action"`
					TableauRequestID string `json:"tableau_request_id"`
					UpstreamStatus   int    `json:"upstream_status"`
					Retryable        *bool  `json:"retryable"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(output.String()), &cloudDocument); err != nil {
				t.Fatal(err)
			}
			if code == 0 || cloudDocument.Error.ID != "search.operation" || cloudDocument.Error.UpstreamStatus != status || cloudDocument.Error.TableauRequestID != "metric-endpoint-unavailable" || cloudDocument.Error.Retryable == nil || *cloudDocument.Error.Retryable || nativeCalls.Load() != 1 {
				t.Fatalf("unexpected Cloud failure: code=%d calls=%d output=%s", code, nativeCalls.Load(), output.String())
			}
			advice := cloudDocument.Error.CorrectiveAction
			if !strings.Contains(advice, "Tableau Cloud") || !strings.Contains(advice, "Pulse") || strings.Contains(advice, "Tableau Server") {
				t.Errorf("Cloud-only metric search has invalid recovery advice: %q", advice)
			}
			output.Reset()
			serverCode := app.Run(t.Context(), args, &output, diagnosticOptions(t, server))
			var serverDocument struct {
				Error struct {
					ID string `json:"id"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(output.String()), &serverDocument); err != nil {
				t.Fatal(err)
			}
			if serverCode == 0 || serverDocument.Error.ID != "search.unsupported_type" || nativeCalls.Load() != 1 {
				t.Fatalf("Server should reject metric before native request: code=%d calls=%d output=%s", serverCode, nativeCalls.Load(), output.String())
			}
		})
	}
}
