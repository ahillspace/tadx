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

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestPulseBundlePullPublishPreservesSpecificationsThroughCLI(t *testing.T) {
	for _, site := range []string{"site-1", "site-2"} {
		t.Run(site, func(t *testing.T) { testPulseBundleRoundTrip(t, site, pulseBundleReadbackChange{}) })
	}
}

type pulseBundleReadbackChange struct {
	sourceOld, sourceNew     string
	readbackOld, readbackNew string
	mismatchSection          string
}

func TestPulseBundlePublishReadbackSemanticsThroughCLI(t *testing.T) {
	const numericComparison = `"comparisons":[{"index":1,"compare_config":{"comparison":"TIME_COMPARISON_YEAR_AGO_PERIOD"}}]`
	const quotedComparison = `"comparisons":[{"index":"1","compare_config":{"comparison":"TIME_COMPARISON_YEAR_AGO_PERIOD"}}]`
	const thresholdGoal = `"datasource_goals":[{"name":"Target","threshold_basic_specification":{"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"},{"string_value":"East"}],"include_null":true}]}}]`
	const values = `"categorical_values":[{"string_value":"West"},{"string_value":"East"}]`
	for _, test := range []struct {
		name   string
		change pulseBundleReadbackChange
	}{
		{"numeric_index_read_as_string", pulseBundleReadbackChange{`"comparisons":[]`, numericComparison, `"index":1`, `"index":"1"`, ""}},
		{"quoted_index_read_as_number", pulseBundleReadbackChange{`"comparisons":[]`, quotedComparison, `"index":"1"`, `"index":1`, ""}},
		{"different_index", pulseBundleReadbackChange{`"comparisons":[]`, numericComparison, `"index":1`, `"index":"2"`, "comparisons"}},
		{"reordered_threshold_values", pulseBundleReadbackChange{`"datasource_goals":[]`, thresholdGoal, values, `"categorical_values":[{"string_value":"East"},{"string_value":"West"}]`, ""}},
		{"different_threshold_value", pulseBundleReadbackChange{`"datasource_goals":[]`, thresholdGoal, values, `"categorical_values":[{"string_value":"North"},{"string_value":"East"}]`, "datasource_goals"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			testPulseBundleRoundTrip(t, "site-1", test.change)
		})
	}
}

func testPulseBundleRoundTrip(t *testing.T, destinationSite string, change pulseBundleReadbackChange) {
	t.Helper()
	definition := `{"metadata":{"id":"source-definition","name":"Revenue","description":"Portable"},"specification":{"datasource":{"id":"source-ds"},"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"},"filters":[{"field":"Region","operator":"OPERATOR_NOT_EQUAL","categorical_values":[{"string_value":"East"}],"include_null":true}]},"is_running_total":false,"temporality":"TEMPORALITY_OVER_TIME"},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_DAY","GRANULARITY_BY_MONTH"],"offset_from_today":2,"use_dynamic_offset":true},"representation_options":{"type":"NUMBER_FORMAT_TYPE_NUMBER","sentiment_type":"SENTIMENT_TYPE_UP_IS_GOOD"},"insights_options":{"show_insights":true,"settings":[]},"comparisons":{"comparisons":[]},"datasource_goals":[],"related_links":[],"certification":{"is_certified":false}}`
	if change.sourceOld != "" {
		if strings.Count(definition, change.sourceOld) != 1 {
			t.Fatal("source fixture edit must match exactly once")
		}
		definition = strings.Replace(definition, change.sourceOld, change.sourceNew, 1)
	}
	specs := []map[string]any{
		{"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_MONTH", "range": "RANGE_CURRENT_PARTIAL"}, "filters": []any{}},
		{"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_LAST_N", "last_n": float64(17), "offset": float64(3)}, "filters": []any{map[string]any{"field": "Region", "operator": "OPERATOR_NOT_EQUAL", "categorical_values": []any{map[string]any{"string_value": "West"}, map[string]any{"null_value": "NULL_VALUE"}}, "include_null": true}}},
	}
	var created map[string]any
	saved := map[string]map[string]any{}
	posts := 0
	definitionReads := 0
	requests := 0
	currentSite := "site-1"
	invalidFields, incompletePull := false, false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case r.URL.Path == "/api/3.29/auth/signin":
			_, _ = fmt.Fprintf(w, `{"credentials":{"token":"fixture-session","site":{"id":%q},"user":{"id":"user-1"}}}`, currentSite)
		case r.URL.Path == "/api/-/pulse/definitions/source-definition":
			_, _ = io.WriteString(w, definition)
		case r.URL.Path == "/api/-/pulse/definitions/source-definition/metrics":
			if incompletePull {
				_, _ = io.WriteString(w, `{"metrics":[],"next_page_token":"same-token"}`)
				return
			}
			_, _ = io.WriteString(w, `{"metrics":[{"id":"source-default","definition_id":"source-definition","is_default":true},{"id":"source-variant","definition_id":"source-definition","is_default":false}]}`)
		case r.URL.Path == "/api/-/pulse/metrics/source-default" || r.URL.Path == "/api/-/pulse/metrics/source-variant":
			i := 0
			if strings.HasSuffix(r.URL.Path, "source-variant") {
				i = 1
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": []string{"source-default", "source-variant"}[i], "definition_id": "source-definition", "site_id": "site-1", "is_default": i == 0, "specification": specs[i]})
		case r.URL.Path == "/api/3.29/sites/"+destinationSite+"/datasources/destination-ds":
			_, _ = io.WriteString(w, `<tsResponse><datasource id="destination-ds" name="Orders"><project id="project-1" name="Test"/></datasource></tsResponse>`)
		case r.URL.Path == "/api/v1/vizql-data-service/read-metadata":
			if invalidFields {
				_, _ = io.WriteString(w, `{"data":[{"fieldName":"Revenue","dataType":"REAL","fieldRole":"MEASURE"},{"fieldName":"Order Date","dataType":"DATE","fieldRole":"DIMENSION"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":[{"fieldName":"Revenue","dataType":"REAL","fieldRole":"MEASURE"},{"fieldName":"Order Date","dataType":"DATE","fieldRole":"DIMENSION"},{"fieldName":"Region","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/-/pulse/definitions":
			if r.Header.Get("X-Tableau-Site-Id") != destinationSite {
				t.Errorf("wrong definition target site: %s", r.Header.Get("X-Tableau-Site-Id"))
			}
			posts++
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(201)
			_, _ = io.WriteString(w, `{"definition":{"metadata":{"id":"destination-definition"}}}`)
		case r.URL.Path == "/api/-/pulse/definitions/destination-definition/metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{"metrics": []any{map[string]any{"id": "destination-default", "definition_id": "destination-definition", "is_default": true, "specification": specs[0]}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/-/pulse/metrics:getOrCreate":
			posts++
			var body struct {
				Definition    string         `json:"definition_id"`
				Specification map[string]any `json:"specification"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Definition != "destination-definition" {
				t.Errorf("wrong target %q", body.Definition)
			}
			id := fmt.Sprintf("destination-metric-%d", len(saved))
			if reflect.DeepEqual(body.Specification, specs[0]) {
				id = "destination-default"
			}
			saved[id] = body.Specification
			_ = json.NewEncoder(w).Encode(map[string]any{"metric": map[string]any{"id": id}, "is_metric_created": true})
		case r.URL.Path == "/api/-/pulse/definitions/destination-definition":
			definitionReads++
			readback := make(map[string]any, len(created)+1)
			for key, value := range created {
				readback[key] = value
			}
			delete(readback, "name")
			delete(readback, "description")
			readback["metadata"] = map[string]any{"id": "destination-definition", "name": created["name"], "description": created["description"]}
			data, err := json.Marshal(readback)
			if err != nil {
				t.Errorf("encode saved definition: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if change.readbackOld != "" {
				if bytes.Count(data, []byte(change.readbackOld)) != 1 {
					t.Errorf("readback fixture edit must match exactly once: %s", data)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				data = bytes.Replace(data, []byte(change.readbackOld), []byte(change.readbackNew), 1)
			}
			_, _ = w.Write(data)
		case strings.HasPrefix(r.URL.Path, "/api/-/pulse/metrics/destination-metric-") || r.URL.Path == "/api/-/pulse/metrics/destination-default":
			id := strings.TrimPrefix(r.URL.Path, "/api/-/pulse/metrics/")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "definition_id": "destination-definition", "site_id": destinationSite, "specification": saved[id]})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	workspace := createNamedWorkspace(t, options.ConfigPath, "portable")
	run := func(args ...string) (int, string) {
		var out bytes.Buffer
		code := app.Run(context.Background(), args, &out, options)
		return code, out.String()
	}
	if code, out := run("pulse", "definition", "pull", "--id", "source-definition", "--workspace", "portable"); code != 0 {
		t.Fatalf("pull=%d %s", code, out)
	}
	paths, _ := filepath.Glob(filepath.Join(workspace, "artifacts", "pulse-definition", "*", "bundle.json"))
	if len(paths) != 1 {
		t.Fatalf("pull did not produce a complete portable bundle: %v", paths)
	}
	bundle, _ := os.ReadFile(paths[0])
	if !bytes.Contains(bundle, []byte("source-variant")) || !bytes.Contains(bundle, []byte("last_n")) {
		t.Fatalf("bundle lost variant specification: %s", bundle)
	}
	incompletePull = true
	if code, out := run("pulse", "definition", "pull", "--id", "source-definition", "--workspace", "portable"); code == 0 {
		t.Fatalf("incomplete pull succeeded: %s", out)
	}
	after, _ := os.ReadFile(paths[0])
	if !bytes.Equal(bundle, after) {
		t.Fatal("incomplete pull damaged previous bundle")
	}
	incompletePull = false
	configuration, err := config.Load(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	environment := configuration.Environments["test"]
	environment.Alias = "destination"
	if destinationSite != "site-1" {
		environment.SiteContentURL = "destination"
	}
	configuration.Environments["destination"] = environment
	if err := config.Save(options.ConfigPath, configuration); err != nil {
		t.Fatal(err)
	}
	currentSite = destinationSite
	rel, _ := filepath.Rel(workspace, filepath.Dir(paths[0]))
	baseArgs := []string{"pulse", "definition", "publish", "--artifact", filepath.ToSlash(rel), "--workspace", "portable", "--environment", "destination"}
	for _, mapping := range [][]string{nil, {"--datasource-map", "other=destination-ds"}, {"--datasource-map", "source-ds=destination-ds", "--datasource-map", "source-ds=duplicate"}} {
		before := requests
		code, out := run(append(append([]string(nil), baseArgs...), mapping...)...)
		if code == 0 || requests != before {
			t.Fatalf("local mapping failure requested Tableau: code=%d delta=%d %s", code, requests-before, out)
		}
	}
	args := append(baseArgs, "--datasource-map", "source-ds=destination-ds")
	invalidFields = true
	if code, out := run(append(args, "--preview")...); code == 0 || posts != 0 {
		t.Fatalf("invalid field accepted: code=%d posts=%d %s", code, posts, out)
	}
	invalidFields = false
	for _, selector := range [][]string{{"--id", "source-definition"}, {"--artifact-name", "Revenue"}} {
		selected := []string{"pulse", "definition", "publish", "--workspace", "portable", "--environment", "destination", "--datasource-map", "source-ds=destination-ds", "--preview"}
		selected = append(selected, selector...)
		if code, out := run(selected...); code != 0 || posts != 0 {
			t.Fatalf("selector preview failed: %d %s", code, out)
		}
	}
	if code, out := run(append(args, "--preview")...); code != 0 || posts != 0 || !strings.Contains(out, "destination-ds") || !strings.Contains(out, "source-variant") {
		t.Fatalf("preview=%d posts=%d %s", code, posts, out)
	}
	code, out := run(append(args, "--full")...)
	if change.mismatchSection != "" {
		if code == 0 || posts != 1 || definitionReads != 1 || len(saved) != 0 ||
			!strings.Contains(out, "definition_reconciliation") || !strings.Contains(out, change.mismatchSection) ||
			!strings.Contains(out, "status: partial") || !strings.Contains(out, "complete: false") ||
			!strings.Contains(out, "destination-definition") {
			t.Fatalf("mismatch code=%d posts=%d definition reads=%d metrics=%d output=%s", code, posts, definitionReads, len(saved), out)
		}
		return
	}
	if code != 0 || !strings.Contains(out, "destination-definition") || !strings.Contains(out, "source-variant") {
		t.Fatalf("publish=%d %s", code, out)
	}
	if len(saved) != 2 || !reflect.DeepEqual(saved["destination-default"], specs[0]) || !reflect.DeepEqual(saved["destination-metric-1"], specs[1]) {
		t.Fatalf("specifications changed: %#v", saved)
	}
	if definitionReads != 1 {
		t.Fatalf("definition readbacks=%d, want one shared immutable readback", definitionReads)
	}
	var original map[string]any
	_ = json.Unmarshal([]byte(definition), &original)
	originalSpec := original["specification"].(map[string]any)
	originalSpec["datasource"].(map[string]any)["id"] = "destination-ds"
	if !reflect.DeepEqual(created["specification"], originalSpec) || !reflect.DeepEqual(created["extension_options"], original["extension_options"]) {
		t.Fatalf("definition changed: %#v", created)
	}
	if !reflect.DeepEqual(created["comparisons"], original["comparisons"]) || !reflect.DeepEqual(created["datasource_goals"], original["datasource_goals"]) {
		t.Fatalf("submitted comparison or goal configuration changed: %#v", created)
	}
}
