package app_test

import (
	"bytes"
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
			var metrics [3]atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(405)
					return
				}
				switch r.URL.Path {
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
					index, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/-/pulse/metrics/metric-"))
					if err != nil || index < 1 || index > len(metrics) {
						t.Errorf("unexpected request %s", r.URL.Path)
						w.WriteHeader(404)
						return
					}
					metrics[index-1].Add(1)
					fmt.Fprintf(w, `{"id":"metric-%d","definition_id":"definition-1","site_id":"site-1","is_default":%t,"specification":{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_LAST_N","last_n":%d},"filters":[]}}`, index, index == 1, index)
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
			t.Logf("definition reads=%d page reads=%d+%d metric detail reads=%d+%d+%d", definitions.Load(), firstPages.Load(), secondPages.Load(), metrics[0].Load(), metrics[1].Load(), metrics[2].Load())
			if definitions.Load() != wantDefinitions || firstPages.Load() != 1 || secondPages.Load() != 1 || metrics[0].Load() != 1 || metrics[1].Load() != 1 || metrics[2].Load() != 1 {
				t.Fatalf("saved metric bundle was reacquired; definition reads=%d want=%d page reads=%d+%d metric reads=%d+%d+%d", definitions.Load(), wantDefinitions, firstPages.Load(), secondPages.Load(), metrics[0].Load(), metrics[1].Load(), metrics[2].Load())
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
			}
		})
	}
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
