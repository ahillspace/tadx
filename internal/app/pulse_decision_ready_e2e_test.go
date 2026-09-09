package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestPulseCreateCompactPreviewShowsConsequentialSettingsThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/datasources/datasource-1":
			_, _ = io.WriteString(w, `<tsResponse><datasource id="datasource-1" name="Revenue"><project id="project-1" name="Test"/></datasource></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			_, _ = io.WriteString(w, `{"data":[{"fieldName":"Sales","dataType":"REAL","fieldRole":"MEASURE"},{"fieldName":"Date","dataType":"DATE","fieldRole":"DIMENSION"},{"fieldName":"Region","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
		case "/api/-/pulse/definitions":
			if r.Method != http.MethodGet {
				t.Error("preview attempted write")
			}
			_, _ = io.WriteString(w, `{"definitions":[]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "definition", "create", "--environment", "test", "--name", "Revenue", "--datasource-id", "datasource-1", "--measure-field", "Sales", "--date-field", "Date", "--dimension", "Region", "--minimum-granularity", "MONTH", "--number-format", "CURRENCY", "--currency", "EUR", "--sentiment", "DOWN", "--temporality", "OVER_TIME", "--running-total", "--preview"}, &out, pulseEfficiencyOptions(t, server))
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	for _, want := range []string{"environment: test", "site: test", "minimum_granularity: MONTH", "number_format: CURRENCY", "currency: EUR", "sentiment: DOWN", "temporality: OVER_TIME", "running_total: true", "population: ALL_ROWS"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "extension_options:") || strings.Contains(out.String(), "request:") {
		t.Fatalf("provider envelope in compact preview: %s", out.String())
	}
}

func TestPulseForkCompactPreviewIncludesInheritedPopulationThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/metrics/source":
			_, _ = io.WriteString(w, `{"id":"source","definition_id":"definition-1","specification":{"measurement_period":{"granularity":"GRANULARITY_BY_MONTH","range":"RANGE_LAST_COMPLETE"},"filters":[{"field":"Segment","operator":"OPERATOR_NOT_EQUAL","categorical_values":[{"string_value":"Consumer"}],"include_null":true}]}}`)
		case "/api/-/pulse/definitions/definition-1":
			_, _ = io.WriteString(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"filters":[{"field":"Country","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"Canada"}],"include_null":false}]}},"extension_options":{"allowed_dimensions":["Region","Segment"],"allowed_granularities":["GRANULARITY_BY_MONTH"]}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "metric", "fork", "--environment", "test", "--id", "source", "--filter", "Region=West", "--preview"}, &out, pulseEfficiencyOptions(t, server))
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	for _, want := range []string{"environment: test", "source_metric_luid: source", "population:", "Segment", "NOT_EQUAL", "Consumer", "INCLUDED", "Region", "West", "EXCLUDED", "Country", "Canada", "DEFINITION_FIXED", "review_complete: true", "granularity: MONTH", "range: LAST_COMPLETE"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "specification:") || strings.Contains(out.String(), "categorical_values:") {
		t.Fatalf("provider envelope in compact preview: %s", out.String())
	}
}

func TestPulseConfirmedCreateSurvivesDefaultMetricFailureThroughCLI(t *testing.T) {
	writes := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/datasources/datasource-1":
			_, _ = io.WriteString(w, `<tsResponse><datasource id="datasource-1" name="Revenue"><project id="project-1" name="Test"/></datasource></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			_, _ = io.WriteString(w, `{"data":[{"fieldName":"Sales","dataType":"REAL","fieldRole":"MEASURE"},{"fieldName":"Date","dataType":"DATE","fieldRole":"DIMENSION"},{"fieldName":"Region","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
		case "/api/-/pulse/definitions":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"definitions":[]}`)
				return
			}
			writes++
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"definition":{"metadata":{"id":"created-definition"}}}`)
		case "/api/-/pulse/definitions/created-definition/metrics":
			w.WriteHeader(http.StatusForbidden)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "definition", "create", "--environment", "test", "--name", "Revenue", "--datasource-id", "datasource-1", "--measure-field", "Sales", "--date-field", "Date", "--dimension", "Region"}, &out, pulseEfficiencyOptions(t, server))
	if code == 0 || writes != 1 {
		t.Fatalf("code=%d writes=%d output=%s", code, writes, out.String())
	}
	for _, want := range []string{"status: created", "definition_luid: created-definition", "default_metric_status: unresolved", "retryable: false", "--environment test", "--id created-definition"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}

func TestPulseFollowupKeepsNondefaultEnvironmentAndQuotesExactIdentityThroughCLI(t *testing.T) {
	alias, id := "review team's $literal", "metric'$(literal)"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/3.29/auth/signin" {
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "definition_id": "definition-1", "specification": map[string]any{"filters": []any{}}})
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	data, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	config += "\n  \"" + alias + "\":\n" + config[strings.Index(config, "    url:"):]
	if err := os.WriteFile(options.ConfigPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "metric", "inspect", "--environment", alias, "--id", id}, &out, options)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	decoded, err := toon.Decode(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(decoded)
	var result struct {
		Help []string `json:"help"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	want := commandhint.Environment(alias, "pulse", "metric", "fork", "--id", id, "--period", "LAST_30_DAYS", "--preview")
	if len(result.Help) != 1 || result.Help[0] != want {
		t.Fatalf("help=%v want=%q", result.Help, want)
	}
}
