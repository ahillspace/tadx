package suite

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestPulseInspectRejectsSavedAggregationDrift(t *testing.T) {
	binary := contentCandidate(t)
	for _, aggregation := range []string{"AGGREGATION_SUM", "AGGREGATION_AVERAGE"} {
		t.Run(aggregation, func(t *testing.T) {
			reads := 0
			definition := map[string]any{"metadata": map[string]any{"id": "definition-a", "name": "tadx-live-case-definition", "description": "Business description"}, "specification": map[string]any{"datasource": map[string]any{"id": "datasource-a"}, "basic_specification": map[string]any{"measure": map[string]any{"field": "revenue", "aggregation": "AGGREGATION_SUM"}, "time_dimension": map[string]any{"field": "date"}, "filters": []any{}}}, "extension_options": map[string]any{"allowed_dimensions": []any{"region"}, "allowed_granularities": []any{"GRANULARITY_BY_DAY"}}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "POST" && r.URL.Path == pulseBase+"/definitions":
					w.WriteHeader(201)
					_ = json.NewEncoder(w).Encode(map[string]any{"definition": definition})
				case r.Method == "GET" && r.URL.Path == pulseBase+"/definitions/definition-a":
					reads++
					_ = json.NewEncoder(w).Encode(map[string]any{"definition": definition})
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/definition-a/metrics"):
					_ = json.NewEncoder(w).Encode(map[string]any{"metrics": []any{map[string]any{"id": "default-a", "definition_id": "definition-a", "is_default": true, "specification": map[string]any{"measurement_period": pulsePeriod(30), "filters": []any{}}}}})
				default:
					t.Errorf("unexpected independent request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			row := map[string]any{"luid": "definition-a", "name": "tadx-live-case-definition", "datasource_luid": "datasource-a", "measure_field": "revenue", "aggregation": aggregation, "time_dimension": "date", "allowed_dimensions": []any{"region"}, "allowed_granularities": []any{"GRANULARITY_BY_DAY"}, "configuration": definition}
			data, _ := json.Marshal(map[string]any{"definition": row})
			c := testAdminCase(t, server, binary, string(data))
			c.Profile.Bindings = map[string]harness.Binding{"pulse_datasource": {ID: "datasource-a", Params: map[string]string{"measure_field": "revenue", "date_field": "date", "dimension_field": "region"}}}
			err := pulseDefinition("inspect")(t.Context(), c)
			if (err == nil) != (aggregation == "AGGREGATION_SUM") {
				t.Fatalf("aggregation %s: %v", aggregation, err)
			}
			if reads != 2 {
				t.Fatalf("independent definition observations=%d", reads)
			}
		})
	}
}

func TestPulseBundleOracleRejectsMissingMetricAndExcludedFilterDrift(t *testing.T) {
	definition := map[string]any{"metadata": map[string]any{"id": "d", "name": "fixture"}, "specification": map[string]any{"datasource": map[string]any{"id": "ds"}}}
	spec := map[string]any{"measurement_period": pulsePeriod(14), "filters": []any{pulseFilter("region", "West", true)}}
	metric := map[string]any{"id": "m", "definition_id": "d", "is_default": false, "specification": spec}
	bundle := map[string]any{"definition_luid": "d", "datasource_references": []any{"ds"}, "definition": definition, "metrics": []any{map[string]any{"luid": "m", "definition_luid": "d", "is_default": false, "specification": spec}}}
	if err := verifyPulseBundle(bundle, definition, []any{metric}, "d", "ds"); err != nil {
		t.Fatal(err)
	}
	broken := cloneObject(bundle)
	asObject(array(broken["metrics"])[0])["specification"] = map[string]any{"measurement_period": pulsePeriod(14), "filters": []any{pulseFilter("region", "West", false)}}
	if verifyPulseBundle(broken, definition, []any{metric}, "d", "ds") == nil {
		t.Fatal("included filter satisfied excluded-filter fixture")
	}
	broken = cloneObject(bundle)
	broken["metrics"] = []any{}
	if verifyPulseBundle(broken, definition, []any{metric}, "d", "ds") == nil {
		t.Fatal("empty bundle passed complete metric inventory assertion")
	}
}

func TestPulseMetricFixtureObservesVisibilityWithoutRepeatingCreation(t *testing.T) {
	spec := map[string]any{"measurement_period": pulsePeriod(7), "filters": []any{}}
	submissions, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == pulseBase+"/metrics:getOrCreate":
			submissions++
			_ = json.NewEncoder(w).Encode(map[string]any{"metric": map[string]any{"id": "metric-a"}})
		case r.Method == "GET" && r.URL.Path == pulseBase+"/metrics/metric-a":
			reads++
			if reads == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metric": map[string]any{"id": "metric-a", "definition_id": "definition-a", "specification": spec}})
		default:
			t.Errorf("unexpected fixture request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	c := testAdminCase(t, server, "", "")
	c.Profile.Budgets.ObservationSeconds = 2
	metric, err := pulseSeedMetric(t.Context(), c, "definition-a", spec, "visibility")
	if err != nil {
		t.Fatal(err)
	}
	if pulseID(metric) != "metric-a" || submissions != 1 || reads != 2 {
		t.Fatalf("fixture retry contract: id=%s submissions=%d reads=%d", pulseID(metric), submissions, reads)
	}
	entries := c.Journal.Entries
	if len(entries) != 1 || entries[0].Object == nil || entries[0].Object.ID != "metric-a" {
		t.Fatalf("accepted identity missing from journal: %+v", entries)
	}
}
