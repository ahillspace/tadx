package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestPulseCampaignPulledObservationsRoundTripThroughCLI(t *testing.T) {
	testPulseBundleRoundTrip(t, "site-1", pulseBundleReadbackChange{
		sourceOld: `"certification":{"is_certified":false}`,
		sourceNew: `"certification":{"is_certified":false},"metrics":[],"tags":[],"total_metrics":"2"`,
	})
}

func TestPulseCampaignUnrepresentedObservationsRejectLocallyThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name, section string
		value         any
	}{
		{"populated metrics", "metrics", []any{map[string]any{"id": "source-default"}}},
		{"populated tags", "tags", []any{"important"}},
		{"mismatched total", "total_metrics", "3"},
		{"unknown empty section", "unknown_observation", []any{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPulsePreflightFixture(t)
			path := filepath.Join(fixture.directory, "bundle.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var bundle map[string]any
			if err := json.Unmarshal(data, &bundle); err != nil {
				t.Fatal(err)
			}
			definition := bundle["definition"].(map[string]any)
			definition[test.section] = test.value
			resource, err := json.Marshal(definition)
			if err != nil {
				t.Fatal(err)
			}
			data, err = json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(fixture.directory, "resource.json"), resource, 0o600); err != nil {
				t.Fatal(err)
			}
			fixture.deny.Store(true)
			for _, preview := range []bool{true, false} {
				args := append([]string(nil), fixture.args...)
				if preview {
					args = append(args, "--preview")
				}
				var output bytes.Buffer
				code := app.Run(t.Context(), args, &output, fixture.options)
				if code == 0 || fixture.requests.Load() != 0 || !strings.Contains(output.String(), "pulse.definition.publish.configuration") || !strings.Contains(output.String(), "outcome: not_attempted") {
					t.Fatalf("preview=%t code=%d requests=%d output=%s", preview, code, fixture.requests.Load(), output.String())
				}
			}
		})
	}
}

func TestPulseCampaignForkPreservesNullPopulationThroughCLI(t *testing.T) {
	const sourceSpec = `{"measurement_period":{"granularity":"GRANULARITY_BY_MONTH","range":"RANGE_CURRENT_PARTIAL"},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"null_value":null}],"values":["West"],"include_null":true}],"comparison":{"comparison":"TIME_COMPARISON_PREVIOUS_PERIOD"}}`
	for _, mode := range []string{"preserved", "lost null", "false flag", "conflicting mirror", "extra null attribute"} {
		t.Run(mode, func(t *testing.T) {
			var submitted map[string]any
			writes := 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/3.29/auth/signin":
					_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case "/api/-/pulse/metrics/source":
					_, _ = io.WriteString(w, `{"id":"source","definition_id":"definition-1","specification":`+sourceSpec+`}`)
				case "/api/-/pulse/definitions/definition-1":
					_, _ = io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"filters":[]}},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_MONTH"]}}`)
				case "/api/-/pulse/metrics:getOrCreate":
					writes++
					var body struct {
						Specification map[string]any `json:"specification"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					submitted = body.Specification
					_, _ = io.WriteString(w, `{"metric":{"id":"saved-variant"},"is_metric_created":true}`)
				case "/api/-/pulse/metrics/saved-variant":
					encoded, err := json.Marshal(submitted)
					if err != nil {
						t.Error(err)
					}
					var saved map[string]any
					if err := json.Unmarshal(encoded, &saved); err != nil {
						t.Error(err)
					}
					filter := saved["filters"].([]any)[0].(map[string]any)
					switch mode {
					case "lost null":
						filter["categorical_values"] = []any{map[string]any{"string_value": "West"}}
					case "false flag":
						filter["include_null"] = false
					case "conflicting mirror":
						filter["values"] = []any{"East"}
					case "extra null attribute":
						filter["categorical_values"].([]any)[1].(map[string]any)["extra"] = true
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "saved-variant", "definition_id": "definition-1", "site_id": "site-1", "specification": saved})
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			var output bytes.Buffer
			code := app.Run(t.Context(), []string{"pulse", "metric", "fork", "--environment", "test", "--id", "source", "--period", "LAST_MONTH", "--full"}, &output, pulseEfficiencyOptions(t, server))
			if writes != 1 {
				t.Fatalf("writes=%d output=%s", writes, output.String())
			}
			var original map[string]any
			if err := json.Unmarshal([]byte(sourceSpec), &original); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original["filters"], submitted["filters"]) || !reflect.DeepEqual(original["comparison"], submitted["comparison"]) || submitted["measurement_period"].(map[string]any)["range"] != "RANGE_LAST_COMPLETE" {
				t.Fatalf("fork changed population or comparison: submitted=%#v", submitted)
			}
			if mode == "preserved" {
				if code != 0 || !strings.Contains(output.String(), "reconciliation_status: verified") || !strings.Contains(output.String(), "null_value: null") {
					t.Fatalf("code=%d output=%s", code, output.String())
				}
			} else if code == 0 || !strings.Contains(output.String(), "specification_mismatch") || !strings.Contains(output.String(), "metric_luid: saved-variant") || !strings.Contains(output.String(), "retryable: false") {
				t.Fatalf("mismatched population accepted or confirmed identity lost: code=%d output=%s", code, output.String())
			}
		})
	}
}
