package fork_test

import (
	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	"strings"
	"testing"
)

func TestCompactPopulationBoundsAndMissingSemanticsAreExplicit(t *testing.T) {
	values := make([]any, 21)
	for i := range values {
		values[i] = map[string]any{"string_value": strings.Repeat("x", 300)}
	}
	filters := make([]any, 26)
	for i := range filters {
		filters[i] = map[string]any{"field": "Region", "operator": "OPERATOR_NOT_EQUAL", "categorical_values": values, "include_null": true}
	}
	source := map[string]any{"filters": filters, "measurement_period": map[string]any{"granularity": "GRANULARITY_BY_MONTH", "range": "RANGE_LAST_COMPLETE"}}
	output := metricfork.Output{Plan: metricfork.Plan{Specification: source, DefinitionFiltersKnown: true}}
	compact := output.CompactOutput().(metricfork.CompactOutput)
	if compact.Plan.ReviewComplete || !compact.Plan.RequiresFull || compact.Plan.Population.FiltersOmitted != 1 || len(compact.Plan.Population.Filters) != 25 {
		t.Fatalf("compact=%#v", compact.Plan)
	}
	filter := compact.Plan.Population.Filters[0]
	if filter.ValuesOmitted != 1 || filter.TruncatedValues != 20 || len(filter.Values) != 20 || filter.Nulls != "INCLUDED" {
		t.Fatalf("filter=%#v", filter)
	}
	if len(source["filters"].([]any)) != 26 || len(values) != 21 || len(values[0].(map[string]any)["string_value"].(string)) != 300 {
		t.Fatal("compact projection mutated the full request")
	}
	output.Plan.Specification = map[string]any{"filters": []any{}, "measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_CURRENT_PARTIAL"}}
	output.Plan.DefinitionFiltersKnown = false
	output.Plan.DefinitionFilters = []any{map[string]any{"field": "Fixed", "operator": "OPERATOR_EQUAL", "categorical_values": []any{map[string]any{"bool_value": false}}, "include_null": false}}
	compact = output.CompactOutput().(metricfork.CompactOutput)
	if compact.Plan.ReviewComplete || compact.Plan.Population.Unrepresented == 0 || compact.Plan.Population.Filters[0].Source != "DEFINITION_FIXED" || compact.Plan.Population.Filters[0].Values[0].Value != "false" {
		t.Fatalf("missing evidence hidden: %#v", compact.Plan)
	}
	output.Plan.Specification["filters"] = []any{map[string]any{"field": "Unknown", "operator": "OPERATOR_CUSTOM", "categorical_values": []any{map[string]any{"future_value": "future"}}}}
	compact = output.CompactOutput().(metricfork.CompactOutput)
	if compact.Plan.ReviewComplete || compact.Plan.Population.Filters[0].Nulls != "UNSPECIFIED" || compact.Plan.Population.Filters[0].Unrepresented < 2 {
		t.Fatalf("unsupported semantics hidden: %#v", compact.Plan)
	}
}
