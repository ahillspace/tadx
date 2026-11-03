package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestPulsePullRejectsIncompleteOrMismatchedDetailBatchThroughCLI(t *testing.T) {
	first, second := pulseEfficiencyMetric(1), pulseEfficiencyMetric(2)
	for _, tc := range []struct {
		name, metrics string
	}{
		{"missing", first},
		{"duplicate", first + "," + first},
		{"unexpected", first + "," + pulseEfficiencyMetric(9)},
		{"different-definition", first + "," + strings.Replace(second, `"definition-1"`, `"different-definition"`, 1)},
		{"missing-specification", first + `,{"id":"metric-2","definition_id":"definition-1"}`},
		{"invalid-specification", first + `,{"id":"metric-2","definition_id":"definition-1","specification":{"measurement_period":{},"filters":[]}}`},
	} {
		for _, preview := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preview=%t", tc.name, preview), func(t *testing.T) {
				batches := 0
				server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if diagnosticSignIn(w, r) {
						return
					}
					switch r.URL.Path {
					case "/api/-/pulse/definitions/definition-1":
						fmt.Fprint(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
					case "/api/-/pulse/definitions/definition-1/metrics":
						fmt.Fprint(w, `{"metrics":[{"id":"metric-1","is_default":true},{"id":"metric-2"}]}`)
					case "/api/-/pulse/metrics:batchGet":
						batches++
						fmt.Fprintf(w, `{"metrics":[%s]}`, tc.metrics)
					default:
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusBadRequest)
					}
				}))
				defer server.Close()
				options := pulseEfficiencyOptions(t, server)
				workspace := createNamedWorkspace(t, options.ConfigPath, "portable")
				args := []string{"pulse", "definition", "pull", "--id", "definition-1", "--workspace", "portable", "--json"}
				if preview {
					args = append(args, "--preview")
				}
				var output bytes.Buffer
				code := app.Run(t.Context(), args, &output, options)
				if code == 0 || batches != 1 {
					t.Fatalf("code=%d batches=%d output=%s", code, batches, &output)
				}
				requirePullIdentityArtifacts(t, workspace, "pulse-definition", 0)
			})
		}
	}
}

func TestPulsePullBatchResponseAndBundleBoundsThroughCLI(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		metrics, pageSize, bytesPerMetric int
		wantSuccess                       bool
	}{
		{"valid-batch-above-single-response-bound", 2, 2, 5 * 1024 * 1024, true},
		{"accumulated-bundle-above-32MiB", 5, 1, 7 * 1024 * 1024, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			batches := 0
			padding := strings.Repeat("x", tc.bytesPerMetric)
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch r.URL.Path {
				case "/api/-/pulse/definitions/definition-1":
					fmt.Fprint(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
				case "/api/-/pulse/definitions/definition-1/metrics":
					start := 0
					if token := r.URL.Query().Get("page_token"); token != "" {
						var err error
						start, err = strconv.Atoi(token)
						if err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
					}
					end := min(start+tc.pageSize, tc.metrics)
					rows := []map[string]any{}
					for i := start; i < end; i++ {
						rows = append(rows, map[string]any{"id": fmt.Sprintf("metric-%d", i+1), "is_default": i == 0})
					}
					response := map[string]any{"metrics": rows}
					if end < tc.metrics {
						response["next_page_token"] = strconv.Itoa(end)
					}
					json.NewEncoder(w).Encode(response)
				case "/api/-/pulse/metrics:batchGet":
					batches++
					var request struct {
						IDs []string `json:"metric_ids"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					rows := []map[string]any{}
					for _, id := range request.IDs {
						rows = append(rows, map[string]any{"id": id, "definition_id": "definition-1", "specification": map[string]any{"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "last_n": 7}, "filters": []any{}, "provider_extension": padding}})
					}
					json.NewEncoder(w).Encode(map[string]any{"metrics": rows})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			options := pulseEfficiencyOptions(t, server)
			workspace := createNamedWorkspace(t, options.ConfigPath, "portable")
			var output bytes.Buffer
			code := app.Run(t.Context(), []string{"pulse", "definition", "pull", "--id", "definition-1", "--workspace", "portable", "--preview", "--json"}, &output, options)
			if (code == 0) != tc.wantSuccess || batches != (tc.metrics+tc.pageSize-1)/tc.pageSize {
				t.Fatalf("code=%d batches=%d output=%s", code, batches, &output)
			}
			if !tc.wantSuccess && !strings.Contains(output.String(), "32 MiB") {
				t.Fatalf("missing bundle bound diagnostic: %s", &output)
			}
			requirePullIdentityArtifacts(t, workspace, "pulse-definition", 0)
		})
	}
}
