package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

func TestPulsePullBatchAcquiresSavedMetricsOnce(t *testing.T) {
	for _, tc := range []struct {
		name           string
		batch, preview bool
	}{
		{"ordinary pull", false, false},
		{"one row batch", true, false},
		{"public preview", false, true},
		{"batch public preview", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var definitions, firstPages, secondPages atomic.Int32
			var individualReads, detailBatches atomic.Int32
			var metrics [3]atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.Method != http.MethodGet && !(r.Method == http.MethodPost && r.URL.Path == "/api/-/pulse/metrics:batchGet") {
					t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(405)
					return
				}
				switch r.URL.Path {
				case "/api/-/pulse/metrics:batchGet":
					detailBatches.Add(1)
					var request struct {
						IDs []string `json:"metric_ids"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.IDs) == 0 || len(request.IDs) > 2 {
						t.Errorf("invalid metric batch: %+v %v", request, err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					details := []json.RawMessage{}
					for i := len(request.IDs) - 1; i >= 0; i-- {
						index, err := strconv.Atoi(strings.TrimPrefix(request.IDs[i], "metric-"))
						if err != nil || index < 1 || index > len(metrics) {
							t.Errorf("invalid metric ID %q", request.IDs[i])
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						metrics[index-1].Add(1)
						details = append(details, json.RawMessage(pulseEfficiencyMetric(index)))
					}
					json.NewEncoder(w).Encode(map[string]any{"metrics": details})
				case "/api/-/pulse/definitions/definition-1":
					definitions.Add(1)
					fmt.Fprint(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
				case "/api/-/pulse/definitions/definition-1/metrics":
					switch r.URL.Query().Get("page_token") {
					case "":
						firstPages.Add(1)
						fmt.Fprint(w, `{"metrics":[{"id":"metric-1","definition_id":"definition-1","is_default":true},{"id":"metric-2","definition_id":"definition-1","is_default":false}],"next_page_token":"second"}`)
					case "second":
						secondPages.Add(1)
						fmt.Fprint(w, `{"metrics":[{"id":"metric-3","definition_id":"definition-1","is_default":false}]}`)
					default:
						t.Errorf("unexpected page token %q", r.URL.Query().Get("page_token"))
						w.WriteHeader(400)
					}
				default:
					individualReads.Add(1)
					index, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/-/pulse/metrics/metric-"))
					if err != nil || index < 1 || index > len(metrics) {
						t.Errorf("unexpected request %s", r.URL.Path)
						w.WriteHeader(404)
						return
					}
					metrics[index-1].Add(1)
					fmt.Fprint(w, pulseEfficiencyMetric(index))
				}
			}))
			defer server.Close()
			options := pulseEfficiencyOptions(t, server)
			workspace := createNamedWorkspace(t, options.ConfigPath, "portable")
			args := []string{"pulse", "definition", "pull", "--id", "definition-1", "--workspace", "portable", "--json"}
			if tc.batch {
				args = append(args, "--batch-file", writePullIdentityBatch(t, `[{}]`))
			}
			if tc.preview {
				args = append(args, "--preview")
			}
			var out bytes.Buffer
			if code := app.Run(t.Context(), args, &out, options); code != 0 {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
			wantDefinitions := int32(1)
			if tc.batch {
				wantDefinitions++
			}
			t.Logf("definition reads=%d page reads=%d+%d individual reads=%d detail batches=%d", definitions.Load(), firstPages.Load(), secondPages.Load(), individualReads.Load(), detailBatches.Load())
			if definitions.Load() != wantDefinitions || firstPages.Load() != 1 || secondPages.Load() != 1 || metrics[0].Load() != 1 || metrics[1].Load() != 1 || metrics[2].Load() != 1 || individualReads.Load() != 0 || detailBatches.Load() != 2 {
				t.Fatalf("saved metric acquisition was not bounded by list pages; individual reads=%d detail batches=%d", individualReads.Load(), detailBatches.Load())
			}
			wantArtifacts := 1
			if tc.preview {
				wantArtifacts = 0
			}
			paths := requirePullIdentityArtifacts(t, workspace, "pulse-definition", wantArtifacts)
			if !tc.preview {
				data, err := os.ReadFile(filepath.Join(paths[0], "bundle.json"))
				if err != nil {
					t.Fatal(err)
				}
				bundle, err := artifact.DecodePulseBundle(data)
				if err != nil || len(bundle.Metrics) != len(metrics) {
					t.Fatalf("saved bundle metrics=%d err=%v", len(bundle.Metrics), err)
				}
				for i, metric := range bundle.Metrics {
					if metric.LUID != fmt.Sprintf("metric-%d", i+1) || metric.IsDefault != (i == 0) {
						t.Fatalf("metric inventory order or default marker changed: %+v", bundle.Metrics)
					}
				}
			}
		})
	}
}

func pulseEfficiencyMetric(index int) string {
	return fmt.Sprintf(`{"id":"metric-%d","definition_id":"definition-1","site_id":"site-1","is_default":%t,"specification":{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_LAST_N","last_n":%d},"filters":[]}}`, index, index == 1, index)
}

func TestPulsePullBatchRejectsIncompleteDefinitionBeforeMetricReads(t *testing.T) {
	for _, tc := range []struct{ name, definition string }{
		{"mismatched LUID", `{"metadata":{"id":"other-definition","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`},
		{"missing name", `{"metadata":{"id":"definition-1"},"specification":{"datasource":{"id":"datasource-1"}}}`},
		{"missing datasource", `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var metricReads atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.URL.Path == "/api/-/pulse/definitions/definition-1" {
					fmt.Fprint(w, tc.definition)
					return
				}
				metricReads.Add(1)
				w.WriteHeader(http.StatusBadRequest)
			}))
			defer server.Close()
			options := pulseEfficiencyOptions(t, server)
			workspace := createNamedWorkspace(t, options.ConfigPath, "portable")
			var out bytes.Buffer
			code := app.Run(t.Context(), []string{"pulse", "definition", "pull", "--id", "definition-1", "--workspace", "portable", "--batch-file", writePullIdentityBatch(t, `[{}]`), "--json"}, &out, options)
			if code == 0 || metricReads.Load() != 0 || !strings.Contains(out.String(), "definition") {
				t.Fatalf("code=%d metric reads=%d output=%s", code, metricReads.Load(), out.String())
			}
			requirePullIdentityArtifacts(t, workspace, "pulse-definition", 0)
		})
	}
}

func TestPulsePullBatchRetainsMetricCompletenessGate(t *testing.T) {
	for _, preview := range []bool{false, true} {
		t.Run(fmt.Sprintf("preview_%t", preview), func(t *testing.T) {
			var definitions, pages atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch r.URL.Path {
				case "/api/-/pulse/definitions/definition-1":
					definitions.Add(1)
					fmt.Fprint(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
				case "/api/-/pulse/definitions/definition-1/metrics":
					pages.Add(1)
					fmt.Fprint(w, `{"metrics":[]}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			options := pulseEfficiencyOptions(t, server)
			workspace := createNamedWorkspace(t, options.ConfigPath, "portable")
			args := []string{"pulse", "definition", "pull", "--id", "definition-1", "--workspace", "portable", "--batch-file", writePullIdentityBatch(t, `[{}]`), "--json"}
			if preview {
				args = append(args, "--preview")
			}
			var out bytes.Buffer
			code := app.Run(t.Context(), args, &out, options)
			if code == 0 || definitions.Load() != 2 || pages.Load() != 1 || !strings.Contains(out.String(), "pulse.definition.pull.incomplete") {
				t.Fatalf("code=%d definitions=%d pages=%d output=%s", code, definitions.Load(), pages.Load(), out.String())
			}
			requirePullIdentityArtifacts(t, workspace, "pulse-definition", 0)
		})
	}
}
