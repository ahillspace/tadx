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
	"reflect"
	"strings"
	"testing"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	"github.com/ahillspace/tadx/internal/app"
)

func TestPulseCreateValidationThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name        string
		measure     string
		aggregation string
		dimensions  []string
		extra       []string
		wantError   string
	}{
		{name: "dimension count", measure: "Customer ID", aggregation: "COUNT", dimensions: []string{"Region"}},
		{name: "dimension distinct count", measure: "Customer ID", aggregation: "COUNT_DISTINCT", dimensions: []string{"Region"}},
		{name: "stable dimension order", measure: "Revenue", aggregation: "SUM", dimensions: []string{"Region", " Category ", "Region"}},
		{name: "missing dimensions", measure: "Revenue", aggregation: "SUM", wantError: "at least one adjustable dimension"},
		{name: "text measure minimum unchanged", measure: "Text Measure", aggregation: "MIN", dimensions: []string{"Region"}},
		{name: "text measure maximum unchanged", measure: "Text Measure", aggregation: "MAX", dimensions: []string{"Region"}},
		{name: "numeric minimum", measure: "Revenue", aggregation: "MIN", dimensions: []string{"Region"}},
		{name: "numeric maximum", measure: "Revenue", aggregation: "MAX", dimensions: []string{"Region"}},
		{name: "excluded count", measure: "Hidden ID", aggregation: "COUNT", dimensions: []string{"Region"}, wantError: "excluded"},
		{name: "table calculation count", measure: "Rank", aggregation: "COUNT_DISTINCT", dimensions: []string{"Region"}, wantError: "table calculations cannot be used"},
		{name: "aggregate calculation count", measure: "Aggregate", aggregation: "COUNT", dimensions: []string{"Region"}, wantError: "already aggregated"},
		{name: "ambiguous count", measure: "Duplicate ID", aggregation: "COUNT", dimensions: []string{"Region"}, wantError: "ambiguous raw identity"},
		{name: "unknown count", measure: "Missing ID", aggregation: "COUNT", dimensions: []string{"Region"}, wantError: "was not found in datasource schema"},
		{name: "excluded adjustable dimension", measure: "Revenue", aggregation: "SUM", dimensions: []string{"Hidden ID"}, wantError: "excluded"},
		{name: "running total sum", measure: "Revenue", aggregation: "SUM", dimensions: []string{"Region"}, extra: []string{"--running-total", "--temporality", "OVER_TIME"}},
		{name: "running total average", measure: "Revenue", aggregation: "AVERAGE", dimensions: []string{"Region"}, extra: []string{"--running-total"}, wantError: "running total requires SUM aggregation and OVER_TIME temporality"},
		{name: "running total latest", measure: "Revenue", aggregation: "SUM", dimensions: []string{"Region"}, extra: []string{"--running-total", "--temporality", "LATEST"}, wantError: "running total requires SUM aggregation and OVER_TIME temporality"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var creates, fieldReads, collisionReads int
			var created definitioncreate.CreateRequest
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1":
					_, _ = io.WriteString(w, `<tsResponse><datasource id="ds-1" name="Orders"><project id="project-1" name="Test"/></datasource></tsResponse>`)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/vizql-data-service/read-metadata":
					fieldReads++
					_, _ = io.WriteString(w, `{"data":[
						{"fieldName":"Revenue","dataType":"REAL","fieldRole":"MEASURE"},
						{"fieldName":"Text Measure","dataType":"STRING","fieldRole":"MEASURE"},
						{"fieldName":"Customer ID","dataType":"STRING","fieldRole":"DIMENSION"},
						{"fieldName":"Order Date","dataType":"DATE","fieldRole":"DIMENSION"},
						{"fieldName":"Region","dataType":"STRING","fieldRole":"DIMENSION"},
						{"fieldName":"Category","dataType":"STRING","fieldRole":"DIMENSION"},
						{"fieldName":"Hidden ID","dataType":"STRING","fieldRole":"DIMENSION","isHidden":true},
						{"fieldName":"Rank","dataType":"INTEGER","fieldRole":"MEASURE","columnClass":"TABLE_CALCULATION"},
						{"fieldName":"Aggregate","dataType":"REAL","fieldRole":"MEASURE","defaultAggregation":"AGG"},
						{"fieldName":"Duplicate ID","dataType":"STRING","fieldRole":"DIMENSION"},
						{"fieldName":"Duplicate ID","dataType":"STRING","fieldRole":"DIMENSION"}
					]}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/definitions":
					collisionReads++
					_, _ = io.WriteString(w, `{"definitions":[]}`)
				case r.Method == http.MethodPost && r.URL.Path == "/api/-/pulse/definitions":
					creates++
					if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
						t.Error(err)
					}
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, `{"definition":{"metadata":{"id":"definition-1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/-/pulse/definitions/definition-1/metrics":
					_, _ = io.WriteString(w, `{"metrics":[{"metadata":{"id":"metric-1"},"definition_id":"definition-1","is_default":true,"specification":{}}]}`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			configuration := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PULSE_VALIDATION_PAT_NAME\n      pat_secret_env: PULSE_VALIDATION_PAT_SECRET\n", server.URL)
			if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PULSE_VALIDATION_PAT_NAME", "test-pat")
			t.Setenv("PULSE_VALIDATION_PAT_SECRET", "test-secret")
			args := []string{"pulse", "definition", "create", "--environment", "test", "--name", "Metric", "--datasource-id", "ds-1", "--measure-field", test.measure, "--aggregation", test.aggregation, "--date-field", "Order Date", "--full"}
			for _, dimension := range test.dimensions {
				args = append(args, "--dimension", dimension)
			}
			args = append(args, test.extra...)
			options := withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client()}, true)
			var stdout bytes.Buffer
			code := app.Run(context.Background(), append(append([]string(nil), args...), "--preview"), &stdout, options)
			if test.wantError != "" {
				if code == 0 || !strings.Contains(stdout.String(), test.wantError) || creates != 0 {
					t.Fatalf("preview code=%d creates=%d output=%s; want %q", code, creates, stdout.String(), test.wantError)
				}
				stdout.Reset()
				code = app.Run(context.Background(), args, &stdout, options)
				if code == 0 || !strings.Contains(stdout.String(), test.wantError) || creates != 0 {
					t.Fatalf("create code=%d creates=%d output=%s; want %q", code, creates, stdout.String(), test.wantError)
				}
				return
			}
			if code != 0 || creates != 0 {
				t.Fatalf("preview code=%d creates=%d output=%s", code, creates, stdout.String())
			}
			stdout.Reset()
			code = app.Run(context.Background(), args, &stdout, options)
			if code != 0 || creates != 1 {
				t.Fatalf("create code=%d creates=%d output=%s", code, creates, stdout.String())
			}
			if fieldReads != 2 || collisionReads != 2 {
				t.Fatalf("preview plus fresh execute must each validate once: fields=%d collisions=%d", fieldReads, collisionReads)
			}
			wantDimensions := []string{"Region"}
			if test.name == "stable dimension order" {
				wantDimensions = append(wantDimensions, "Category")
			}
			if !reflect.DeepEqual(created.ExtensionOptions.AllowedDimensions, wantDimensions) {
				t.Fatalf("created dimensions=%v, want %v", created.ExtensionOptions.AllowedDimensions, wantDimensions)
			}
			if created.Specification.BasicSpecification.Measure.Field != test.measure || created.Specification.BasicSpecification.Measure.Aggregation != "AGGREGATION_"+test.aggregation {
				t.Fatalf("created measure=%#v", created.Specification.BasicSpecification.Measure)
			}
		})
	}
}
