package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func pulseEfficiencyOptions(t *testing.T, server *httptest.Server) app.Options {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	config := fmt.Sprintf("version: 1\ndefault_environment: test\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PULSE_EFFICIENCY_PAT_NAME\n      pat_secret_env: PULSE_EFFICIENCY_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PULSE_EFFICIENCY_PAT_NAME", "fixture-pat")
	t.Setenv("PULSE_EFFICIENCY_PAT_SECRET", "fixture-secret")
	return app.Options{ConfigPath: path, HTTPClient: server.Client(), MutationsEnabled: true}
}

func TestPulseDefinitionDatasourceFilterCacheMultipageThroughCLI(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(403) }))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	entries := make([]cache.ResourceEntry, 205)
	for i := range entries {
		ds := "other"
		if i >= 200 {
			ds = "datasource-1"
		}
		id := fmt.Sprintf("definition-%03d", i)
		payload, err := json.Marshal(tableaupulse.Definition{LUID: id, Name: id, DatasourceLUID: ds})
		if err != nil {
			t.Fatal(err)
		}
		entries[i] = cache.ResourceEntry{Environment: "test", Site: "test", Kind: "definition", LUID: id, Name: id, Payload: payload, ObservedAt: time.Now().UTC(), Coverage: "summary"}
	}
	if err := targetCacheFixture(t, options.ConfigPath, nil).UpsertResources(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"--limit", "1"}, {"--all"}} {
		var output bytes.Buffer
		code := app.Run(context.Background(), append([]string{"pulse", "definition", "list", "--cache", "--datasource-id", "datasource-1"}, extra...), &output, options)
		if code != 0 || requests != 0 || !strings.Contains(output.String(), "definition-200") || strings.Contains(output.String(), "definition-199") {
			t.Fatalf("code=%d requests=%d output=%s", code, requests, output.String())
		}
		if extra[0] == "--all" && !strings.Contains(output.String(), "returned: 5") {
			t.Fatalf("all output=%s", output.String())
		}
	}
}

func TestPulseCompactMetricRowsHaveStableColumnsThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/definitions/definition-1/metrics":
			if r.URL.Query().Get("page_size") == "1" {
				_, _ = io.WriteString(w, `{"metrics":[{"id":"one","name":"Named","definition_id":"definition-1"}],"next_page_token":"next"}`)
			} else {
				_, _ = io.WriteString(w, `{"metrics":[{"id":"one","name":"Named","definition_id":"definition-1"},{"id":"two","definition_id":"definition-1"}]}`)
			}
		case "/api/-/pulse/metrics/one":
			_, _ = io.WriteString(w, `{"id":"one","definition_id":"definition-1","specification":{}}`)
		case "/api/-/pulse/subscriptions":
			_, _ = io.WriteString(w, `{"subscriptions":[{"id":"sub-one","follower":{"user_id":"user-one","name":"Named"}},{"id":"sub-two","follower":{"user_id":"user-two"}}]}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	for _, extra := range [][]string{nil, {"--limit", "1"}, {"--limit", "25"}} {
		var output bytes.Buffer
		code := app.Run(context.Background(), append([]string{"pulse", "metric", "list", "--definition-id", "definition-1"}, extra...), &output, options)
		if code != 0 || !strings.Contains(output.String(), "{luid,name,is_default}:") {
			t.Fatalf("code=%d output=%s", code, output.String())
		}
	}
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "metric", "followers", "--id", "one"}, &output, options)
	if code != 0 || !strings.Contains(output.String(), "subscriptions[2]{luid,metric_luid,follower_type,follower_luid,follower_name}:") {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
}

func TestPulseForkCompletesFromExactReadbackWithoutInventoryThroughCLI(t *testing.T) {
	var saved map[string]any
	var writes, metricReads, lists int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/metrics/source":
			_, _ = io.WriteString(w, `{"id":"source","definition_id":"definition-1","specification":{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[],"provider_extension":{"offset":9007199254740993}}}`)
		case "/api/-/pulse/definitions/definition-1":
			_, _ = io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_DAY"]}}`)
		case "/api/-/pulse/metrics:getOrCreate":
			writes++
			var body struct {
				Specification map[string]any `json:"specification"`
			}
			decoder := json.NewDecoder(r.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Error(err)
			}
			saved = body.Specification
			_, _ = io.WriteString(w, `{"metric":{"id":"variant-201"},"is_metric_created":true}`)
		case "/api/-/pulse/metrics/variant-201":
			metricReads++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "variant-201", "definition_id": "definition-1", "site_id": "site-1", "specification": saved})
		case "/api/-/pulse/definitions/definition-1/metrics":
			// A real variant beyond the first 100 need never be list-visible to verify it.
			lists++
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = map[string]any{"id": fmt.Sprintf("variant-%d", i), "definition_id": "definition-1"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metrics": items, "next_page_token": "more-variants"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "metric", "fork", "--environment", "test", "--id", "source", "--period", "LAST_30_DAYS", "--filter", "Region=West", "--full"}, &output, pulseEfficiencyOptions(t, server))
	if code != 0 || writes != 1 || metricReads != 1 || lists != 0 || !strings.Contains(output.String(), "reconciliation_status: verified") || !strings.Contains(output.String(), "saved_specification:") || strings.Contains(output.String(), "tadx pulse metric inspect") {
		t.Fatalf("code=%d writes=%d exact=%d lists=%d output=%s", code, writes, metricReads, lists, output.String())
	}
	if !strings.Contains(output.String(), "9007199254740993") {
		t.Fatalf("saved numeric fidelity lost: %s", output.String())
	}
}

func TestPulseDefinitionDatasourceFilterBeforeLimitThroughCLI(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/3.29/auth/signin" {
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		if r.URL.Path != "/api/-/pulse/definitions" {
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		requests++
		if r.URL.Query().Get("page_token") == "" {
			_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"unrelated","name":"Revenue"},"specification":{"datasource":{"id":"other"}}}],"next_page_token":"page-2"}`)
		} else {
			_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"wanted","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}},{"metadata":{"id":"wanted-2","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}]}`)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "definition", "list", "--datasource-id", "datasource-1", "--name", "Revenue", "--limit", "1"}, &output, pulseEfficiencyOptions(t, server))
	if code != 0 || requests != 2 || !strings.Contains(output.String(), "wanted,Revenue,datasource-1") || !strings.Contains(output.String(), "more_available: true") || strings.Contains(output.String(), "unrelated") {
		t.Fatalf("code=%d requests=%d output=%s", code, requests, output.String())
	}
}

func TestPulseForkDoesNotReportSuccessForUnverifiedReadbackThroughCLI(t *testing.T) {
	for _, mode := range []string{"period mismatch", "filter mismatch", "definition mismatch", "unavailable", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var saved map[string]any
			writes := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/3.29/auth/signin":
					_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case "/api/-/pulse/metrics/source":
					_, _ = io.WriteString(w, `{"id":"source","definition_id":"definition-1","specification":{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_CURRENT_PARTIAL"},"filters":[]}}`)
				case "/api/-/pulse/definitions/definition-1":
					_, _ = io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_DAY"]}}`)
				case "/api/-/pulse/metrics:getOrCreate":
					writes++
					var body struct {
						Specification map[string]any `json:"specification"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					saved = body.Specification
					_, _ = io.WriteString(w, `{"metric":{"id":"saved-variant"},"is_metric_created":true}`)
				case "/api/-/pulse/metrics/saved-variant":
					if mode == "cancel" {
						cancel()
						w.WriteHeader(404)
						return
					}
					if mode == "unavailable" {
						w.WriteHeader(403)
						return
					}
					definition := "definition-1"
					switch mode {
					case "period mismatch":
						saved["measurement_period"] = map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_LAST_COMPLETE"}
					case "filter mismatch":
						saved["filters"] = []any{}
					case "definition mismatch":
						definition = "other"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "saved-variant", "definition_id": definition, "specification": saved})
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(403)
				}
			}))
			defer server.Close()
			var output bytes.Buffer
			code := app.Run(ctx, []string{"pulse", "metric", "fork", "--environment", "test", "--id", "source", "--period", "LAST_30_DAYS", "--filter", "Region=West"}, &output, pulseEfficiencyOptions(t, server))
			if code == 0 || writes != 1 || !strings.Contains(output.String(), "resource: saved-variant") || !strings.Contains(output.String(), "retryable: false") || !strings.Contains(output.String(), "result:") || !strings.Contains(output.String(), "metric_luid: saved-variant") || strings.Contains(output.String(), "reconciliation_status: verified") {
				t.Fatalf("code=%d writes=%d output=%s", code, writes, output.String())
			}
		})
	}
}
