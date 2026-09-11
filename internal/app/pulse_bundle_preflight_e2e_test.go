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
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

const preflightDefinition = `{"metadata":{"id":"source-definition","name":"Revenue","description":"Portable"},"specification":{"datasource":{"id":"source-ds"},"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"},"filters":[]},"is_running_total":false,"temporality":"TEMPORALITY_OVER_TIME"},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_DAY","GRANULARITY_BY_MONTH"]},"representation_options":{"type":"NUMBER_FORMAT_TYPE_NUMBER","sentiment_type":"SENTIMENT_TYPE_UP_IS_GOOD"},"insights_options":{"show_insights":true,"settings":[]},"comparisons":{"comparisons":[]},"datasource_goals":[],"related_links":[],"certification":{"is_certified":false}}`

type pulsePreflightFixture struct {
	options                  app.Options
	directory                string
	args                     []string
	requests, signins, posts atomic.Int32
	deny                     atomic.Bool
}

func newPulsePreflightFixture(t *testing.T) *pulsePreflightFixture {
	t.Helper()
	fixture := &pulsePreflightFixture{}
	specs := []string{
		`{"measurement_period":{"granularity":"GRANULARITY_BY_MONTH","range":"RANGE_CURRENT_PARTIAL"},"filters":[]}`,
		`{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_LAST_N","last_n":17,"offset":3},"filters":[{"field":"Region","operator":"OPERATOR_NOT_EQUAL","categorical_values":[{"string_value":"West"},{"null_value":"NULL_VALUE"}],"include_null":true}]}`,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.requests.Add(1)
		if r.Method == http.MethodPost {
			fixture.posts.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			fixture.signins.Add(1)
		}
		if fixture.deny.Load() {
			http.Error(w, "preflight must finish locally", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/definitions/source-definition":
			io.WriteString(w, preflightDefinition)
		case "/api/-/pulse/definitions/source-definition/metrics":
			io.WriteString(w, `{"metrics":[{"id":"source-default","definition_id":"source-definition","is_default":true},{"id":"source-variant","definition_id":"source-definition","is_default":false}]}`)
		case "/api/-/pulse/metrics/source-default", "/api/-/pulse/metrics/source-variant":
			i := 0
			if strings.HasSuffix(r.URL.Path, "source-variant") {
				i = 1
			}
			fmt.Fprintf(w, `{"id":%q,"definition_id":"source-definition","site_id":"site-1","is_default":%t,"specification":%s}`, []string{"source-default", "source-variant"}[i], i == 0, specs[i])
		case "/api/3.29/sites/site-1/datasources/destination-ds":
			io.WriteString(w, `<tsResponse><datasource id="destination-ds" name="Orders"><project id="project-1" name="Test"/></datasource></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			io.WriteString(w, `{"data":[{"fieldName":"Revenue","dataType":"REAL","fieldRole":"MEASURE"},{"fieldName":"Order Date","dataType":"DATE","fieldRole":"DIMENSION"},{"fieldName":"Region","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
		default:
			t.Errorf("unexpected fixture request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	fixture.options = pulseEfficiencyOptions(t, server)
	workspace := createNamedWorkspace(t, fixture.options.ConfigPath, "portable")
	var out bytes.Buffer
	if code := app.Run(context.Background(), []string{"pulse", "definition", "pull", "--id", "source-definition", "--workspace", "portable"}, &out, fixture.options); code != 0 {
		t.Fatalf("pull=%d %s", code, out.String())
	}
	paths, err := filepath.Glob(filepath.Join(workspace, "artifacts", "pulse-definition", "*", "bundle.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("bundlepaths=%v err=%v", paths, err)
	}
	fixture.directory = filepath.Dir(paths[0])
	relative, err := filepath.Rel(workspace, fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	fixture.args = []string{"pulse", "definition", "publish", "--artifact", filepath.ToSlash(relative), "--workspace", "portable", "--environment", "test", "--datasource-map", "source-ds=destination-ds"}
	fixture.requests.Store(0)
	fixture.posts.Store(0)
	fixture.signins.Store(0)
	return fixture
}

func TestPulseBundleInvalidSavedSemanticsRejectBeforeAuthenticationThroughCLI(t *testing.T) {
	cases := []struct {
		name       string
		metric     func(map[string]any)
		definition func(map[string]any)
		period     map[string]any
		diagnostic string
	}{
		{name: "later_metric_categorical_values_string", metric: func(spec map[string]any) { spec["filters"].([]any)[0].(map[string]any)["categorical_values"] = "West" }},
		{name: "later_metric_include_null_string", metric: func(spec map[string]any) { spec["filters"].([]any)[0].(map[string]any)["include_null"] = "true" }},
		{name: "later_metric_period_range_array", metric: func(spec map[string]any) {
			spec["measurement_period"].(map[string]any)["range"] = []any{"RANGE_LAST_N"}
		}},
		{name: "later_metric_last_n_string", metric: func(spec map[string]any) { spec["measurement_period"].(map[string]any)["last_n"] = "17" }},
		{name: "later_metric_config_missing", period: map[string]any{}, diagnostic: "metric.measurement_period requires a saved period configuration"},
		{name: "later_metric_config_offset_only", period: map[string]any{"offset": 0}, diagnostic: "metric.measurement_period requires a saved period configuration"},
		{name: "later_metric_config_last_n_only", period: map[string]any{"last_n": 17}, diagnostic: "metric.measurement_period requires a saved period configuration"},
		{name: "later_metric_config_combined_metadata", period: map[string]any{"offset": 0, "last_n": 17}, diagnostic: "metric.measurement_period requires a saved period configuration"},
		{name: "later_metric_config_null_extension", period: map[string]any{"saved_custom_period": nil}, diagnostic: "metric.measurement_period requires a saved period configuration"},
		{name: "definition_invalid_aggregation", definition: func(def map[string]any) {
			def["specification"].(map[string]any)["basic_specification"].(map[string]any)["measure"].(map[string]any)["aggregation"] = "AGGREGATION_UNSUPPORTED"
		}},
		{name: "definition_running_total_latest_point", diagnostic: "is_running_total: running total requires SUM aggregation and OVER_TIME temporality", definition: func(def map[string]any) {
			spec := def["specification"].(map[string]any)
			spec["is_running_total"] = true
			spec["temporality"] = "TEMPORALITY_LATEST_POINT_IN_TIME"
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPulsePreflightFixture(t)
			if !fixture.options.MutationsEnabled {
				t.Fatal("preflight execution fixture must enable mutations")
			}
			path := filepath.Join(fixture.directory, "bundle.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var bundle map[string]any
			if err := json.Unmarshal(data, &bundle); err != nil {
				t.Fatal(err)
			}
			if test.metric != nil {
				test.metric(bundle["metrics"].([]any)[1].(map[string]any)["specification"].(map[string]any))
			}
			if test.period != nil {
				period := map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG"}
				for key, value := range test.period {
					period[key] = value
				}
				bundle["metrics"].([]any)[1].(map[string]any)["specification"].(map[string]any)["measurement_period"] = period
			}
			if test.definition != nil {
				definition := bundle["definition"].(map[string]any)
				test.definition(definition)
				resource, err := json.Marshal(definition)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(fixture.directory, "resource.json"), resource, 0600); err != nil {
					t.Fatal(err)
				}
			}
			data, err = json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			fixture.deny.Store(true)
			for _, preview := range []bool{true, false} {
				t.Run(fmt.Sprintf("preview_%t", preview), func(t *testing.T) {
					fixture.requests.Store(0)
					fixture.signins.Store(0)
					fixture.posts.Store(0)
					args := append([]string(nil), fixture.args...)
					if preview {
						args = append(args, "--preview")
					}
					var out bytes.Buffer
					code := app.Run(context.Background(), args, &out, fixture.options)
					if code == 0 || fixture.requests.Load() != 0 || fixture.signins.Load() != 0 || fixture.posts.Load() != 0 {
						t.Fatalf("code=%d requests=%d signins=%d POSTs=%d output=%s", code, fixture.requests.Load(), fixture.signins.Load(), fixture.posts.Load(), out.String())
					}
					if test.diagnostic != "" && !strings.Contains(out.String(), test.diagnostic) {
						t.Fatalf("missing diagnostic %q: %s", test.diagnostic, out.String())
					}
				})
			}
		})
	}
}

func TestPulseBundleSupportedPeriodAndNullValuesPreviewThroughCLI(t *testing.T) {
	fixture := newPulsePreflightFixture(t)
	fixture.options.MutationsEnabled = false
	var out bytes.Buffer
	if code := app.Run(context.Background(), append(fixture.args, "--preview"), &out, fixture.options); code != 0 {
		t.Fatalf("valid preview=%d %s", code, out.String())
	}
	if fixture.signins.Load() != 1 || !strings.Contains(out.String(), "source-variant") {
		t.Fatalf("signins%d output%s", fixture.signins.Load(), out.String())
	}
}
