package pulse

import (
	"encoding/json"
	"testing"

	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestMetricProjectionRequiredEmptyFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"metric list", metricListItem(tableaupulse.Metric{}), `{"luid":"","definition_luid":"","is_default":false}`},
		{"metric inspect", MetricObservation(tableaupulse.Metric{}), `{"luid":"","definition_luid":"","is_default":false,"specification":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.value)
			if err != nil || string(got) != tc.want {
				t.Fatalf("JSON = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestMetricInspectConversionPreservesIndependentNestedValues(t *testing.T) {
	original := tableaupulse.Metric{Specification: map[string]any{"nested": map[string]any{"value": "original"}}, Configuration: []byte(`{}`)}
	converted := MetricObservation(original)
	converted.Specification["nested"].(map[string]any)["value"] = "changed"
	converted.Configuration[0] = '['
	if original.Specification["nested"].(map[string]any)["value"] != "original" || string(original.Configuration) != `{}` {
		t.Fatal("metric conversion lost its independent nested values")
	}
}
