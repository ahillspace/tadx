package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestPulseProjectionRequiredEmptyFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"metric list", metricListItem(tableaupulse.Metric{}), `{"luid":"","definition_luid":"","is_default":false}`},
		{"metric inspect", metricGetItem(tableaupulse.Metric{}), `{"luid":"","definition_luid":"","is_default":false,"specification":null}`},
		{"follower snapshot", pulseFollowerSnapshot{Version: 1, MetricLUID: "metric", Subscriptions: []pulsemetric.Subscription{{LUID: "subscription"}}}, `{"version":1,"metric_luid":"metric","subscriptions":[{"luid":"subscription","metric_luid":"","follower_type":"","follower_luid":""}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.value)
			if err != nil || string(got) != tc.want {
				t.Fatalf("JSON = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestPulseLiveObservationKeepsOperationSpecificProjections(t *testing.T) {
	var requests []string
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/definitions/definition-1":
			// The provider preserves unknown raw properties, while inspect decodes them into float64.
			_, _ = io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}},"extension":1e1000}`)
		case "/api/-/pulse/metrics/unknown":
			_, _ = io.WriteString(w, `{"metadata":{"id":"unknown"},"definition_id":"definition-1","specification":{}}`)
		case "/api/-/pulse/metrics/known":
			_, _ = io.WriteString(w, `{"metadata":{"id":"known"},"definition_id":"definition-1","is_default":false,"specification":{}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configuration := fmt.Sprintf("version: 1\ndefault_environment: test\nenvironments:\n  test:\n    url: %s\n    site_content_url: sandbox\n    auth:\n      type: pat\n      pat_name_env: PULSE_PROJECTION_PAT_NAME\n      pat_secret_env: PULSE_PROJECTION_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PULSE_PROJECTION_PAT_NAME", "test-pat")
	t.Setenv("PULSE_PROJECTION_PAT_SECRET", "test-secret")
	runtime, err := newRuntime(Options{ConfigPath: configPath, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	commands := newPulseCommands(runtime)
	if _, err := commands.InspectPulseDefinition(t.Context(), pulsedefinition.InspectInput{LUID: "definition-1"}); err == nil {
		t.Fatal("inspect unexpectedly accepted a configuration number outside float64")
	}
	definition, err := commands.DeletePulseDefinition(t.Context(), pulsedefinition.DeleteInput{LUID: "definition-1", Preview: true})
	if err != nil {
		t.Fatalf("delete acquired inspect's configuration decoding failure: %v", err)
	}
	encoded, err := json.Marshal(definition.Plan.Target)
	if err != nil || string(encoded) != `{"luid":"definition-1","name":"Revenue","datasource_luid":"datasource-1"}` {
		t.Fatalf("definition target = %s, %v", encoded, err)
	}
	for _, id := range []string{"unknown", "known"} {
		output, err := commands.DeletePulseMetric(t.Context(), pulsemetric.DeleteInput{LUID: id, Preview: true})
		if err != nil {
			t.Fatal(err)
		}
		if (output.Plan.Target.IsDefault != nil) != (id == "known") {
			t.Fatalf("%s default = %v", id, output.Plan.Target.IsDefault)
		}
		encoded, err := json.Marshal(output.Plan.Target)
		want := `{"luid":"unknown","definition_luid":"definition-1"}`
		if id == "known" {
			want = `{"luid":"known","definition_luid":"definition-1","is_default":false}`
		}
		if err != nil || string(encoded) != want {
			t.Fatalf("metric target = %s, %v; want %s", encoded, err, want)
		}
	}
	wantRequests := []string{"POST /api/3.29/auth/signin", "GET /api/-/pulse/definitions/definition-1", "GET /api/-/pulse/definitions/definition-1", "GET /api/-/pulse/metrics/unknown", "GET /api/-/pulse/metrics/known"}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("requests = %v; want %v", requests, wantRequests)
	}
}

func TestPulseInspectConversionPreservesCopyAndDecodeBoundaries(t *testing.T) {
	if _, err := definitionGetItem(tableaupulse.Definition{Configuration: []byte(`invalid`)}); err == nil {
		t.Fatal("inspect accepted malformed definition configuration")
	}
	original := tableaupulse.Metric{Specification: map[string]any{"nested": map[string]any{"value": "original"}}, Configuration: []byte(`{}`)}
	converted := metricGetItem(original)
	converted.Specification["nested"].(map[string]any)["value"] = "changed"
	converted.Configuration[0] = '['
	if original.Specification["nested"].(map[string]any)["value"] != "original" || string(original.Configuration) != `{}` {
		t.Fatal("metric conversion lost its independent nested values")
	}
}
