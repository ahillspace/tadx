package pulsecontract_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/pulsecontract"
)

const definition = `{"name":"Revenue","specification":{"datasource":{"id":"ds-1"},"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Date"},"filters":[]},"is_running_total":false,"temporality":"TEMPORALITY_OVER_TIME"},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_DAY"]}}`
const metric = `{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_BY_CONFIG","last_x_period":{"period":30,"period_type":"GRANULARITY_BY_DAY","include_current_period":true}},"filters":[{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":[{"string_value":"West"}],"include_null":true}]}`

func TestRejectsInvalidKnownDefinitionFields(t *testing.T) {
	for _, tc := range []struct{ old, replacement, path string }{
		{`"name":"Revenue"`, `"name":false`, "name"},
		{`"aggregation":"AGGREGATION_SUM"`, `"aggregation":"AGGREGATION_BOGUS"`, "aggregation"},
		{`"aggregation":"AGGREGATION_SUM"`, `"aggregation":32`, "aggregation"},
		{`"field":"Revenue"`, `"field":null`, "field"},
		{`"filters":[]`, `"filters":{}`, "filters"},
		{`"is_running_total":false`, `"is_running_total":"true"`, "is_running_total"},
		{`"allowed_dimensions":["Region"]`, `"allowed_dimensions":[true]`, "allowed_dimensions"},
		{`"allowed_granularities":["GRANULARITY_BY_DAY"]`, `"allowed_granularities":["DAY"]`, "allowed_granularities"},
		{`"temporality":"TEMPORALITY_OVER_TIME"`, `"temporality":"LATEST"`, "temporality"},
	} {
		t.Run(tc.path+tc.replacement, func(t *testing.T) {
			err := pulsecontract.ValidateDefinition(json.RawMessage(strings.Replace(definition, tc.old, tc.replacement, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("error=%v, want path %s", err, tc.path)
			}
		})
	}
	for _, aggregation := range []string{"AGGREGATION_SUM", "AGGREGATION_AVERAGE"} {
		raw := strings.ReplaceAll(definition, `"is_running_total":false`, `"is_running_total":true`)
		raw = strings.ReplaceAll(raw, "TEMPORALITY_OVER_TIME", "TEMPORALITY_LATEST_POINT_IN_TIME")
		raw = strings.ReplaceAll(raw, "AGGREGATION_SUM", aggregation)
		if err := pulsecontract.ValidateDefinition(json.RawMessage(raw)); err == nil {
			t.Fatal("accepted running total with latest-point temporality")
		}
	}
}

func TestRejectsMalformedMetricFiltersAndPeriods(t *testing.T) {
	for _, tc := range []struct{ old, replacement, path string }{
		{`"categorical_values":[{"string_value":"West"}]`, `"categorical_values":"West"`, "categorical_values"},
		{`"include_null":true`, `"include_null":"true"`, "include_null"},
		{`"string_value":"West"`, `"string_value":false`, "string_value"},
		{`"string_value":"West"`, `"bool_value":"true"`, "bool_value"},
		{`"string_value":"West"`, `"null_value":null`, "null_value"},
		{`"string_value":"West"`, `"string_value":"West","bool_value":true`, "categorical_values"},
		{`"operator":"OPERATOR_EQUAL"`, `"operator":"OTHER"`, "operator"},
		{`"granularity":"GRANULARITY_BY_DAY"`, `"granularity":true`, "granularity"},
		{`"range":"RANGE_BY_CONFIG"`, `"range":[]`, "range"},
		{`"period":30`, `"period":0`, "period"},
		{`"period":30`, `"period":2.5`, "period"},
		{`"period":30`, `"period":"30"`, "period"},
		{`"period_type":"GRANULARITY_BY_DAY"`, `"period_type":"DAY"`, "period_type"},
		{`"include_current_period":true`, `"include_current_period":"true"`, "include_current_period"},
		{`"last_x_period":{"period":30,"period_type":"GRANULARITY_BY_DAY","include_current_period":true}`, `"last_x_period":{}`, "period"},
		{`"last_x_period":{"period":30,"period_type":"GRANULARITY_BY_DAY","include_current_period":true}`, `"last_x_period":[]`, "last_x_period"},
	} {
		t.Run(tc.path+tc.replacement, func(t *testing.T) {
			err := pulsecontract.ValidateMetric(json.RawMessage(strings.Replace(metric, tc.old, tc.replacement, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("error=%v, want path %s", err, tc.path)
			}
		})
	}
}

func TestValidSavedShapesRemainUnchanged(t *testing.T) {
	for _, raw := range []string{
		definition,
		strings.ReplaceAll(strings.ReplaceAll(definition, "AGGREGATION_SUM", "AGGREGATION_MEDIAN"), "GRANULARITY_BY_DAY", "GRANULARITY_BY_FISCAL_QUARTER"),
		strings.TrimSuffix(definition, "}") + `,"comparisons":{"comparisons":[{"compare_config":{"comparison":"TIME_COMPARISON_FISCAL_YEAR_AGO_PERIOD","extension":{"value":9007199254740993}},"index":"0"}]},"datasource_goals":[{"name":"Target","extension":{"threshold":9007199254740993}}],"unknown":{"n":9007199254740993}}`,
	} {
		data := json.RawMessage(raw)
		before := bytes.Clone(data)
		if err := pulsecontract.ValidateDefinition(data); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, before) {
			t.Fatal("validation changed saved definition")
		}
	}
	for _, raw := range []string{
		metric,
		`{"measurement_period":{"granularity":"GRANULARITY_BY_MONTH","range":"RANGE_CURRENT_PARTIAL"},"filters":[]}`,
		`{"measurement_period":{"granularity":"GRANULARITY_BY_FISCAL_YEAR","range":"RANGE_LAST_COMPLETE"},"filters":[]}`,
		`{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_LAST_N","last_n":9007199254740993,"offset":3},"filters":[],"extension":{"n":9007199254740993}}`,
		`{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_BY_CONFIG","saved_custom_period":{"start":"2025-01-01"}},"filters":[]}`,
	} {
		data := json.RawMessage(raw)
		before := bytes.Clone(data)
		if err := pulsecontract.ValidateMetric(data); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, before) {
			t.Fatal("validation changed saved metric")
		}
	}
}

func TestRejectsInvalidKnownOptionalSections(t *testing.T) {
	for _, section := range []string{
		`"comparisons":[]`, `"comparisons":{"comparisons":[{"index":false}]}`,
		`"datasource_goals":"goal"`, `"datasource_goals":[{"basic_specification":{"filters":[{"field":"Region","categorical_values":"West"}]}}]`,
		`"representation_options":{"positive_only":"true"}`, `"representation_options":{"decimal_places":2.5}`,
		`"insights_options":{"show_insights":"true"}`, `"insights_options":{"settings":[{"disabled":"true"}]}`,
		`"related_links":[{"link_url":false}]`, `"certification":{"is_certified":"true"}`,
	} {
		raw := strings.TrimSuffix(definition, "}") + "," + section + "}"
		if err := pulsecontract.ValidateDefinition(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", section)
		}
	}
}

func TestRunningTotalContractMatchesSavedPayloadValidation(t *testing.T) {
	for _, aggregation := range []string{"AGGREGATION_SUM", "AGGREGATION_AVERAGE", "AGGREGATION_USER"} {
		for _, temporality := range []string{"TEMPORALITY_OVER_TIME", "TEMPORALITY_LATEST_POINT_IN_TIME"} {
			for _, running := range []bool{false, true} {
				raw := strings.ReplaceAll(definition, "AGGREGATION_SUM", aggregation)
				raw = strings.ReplaceAll(raw, "TEMPORALITY_OVER_TIME", temporality)
				if running {
					raw = strings.ReplaceAll(raw, `"is_running_total":false`, `"is_running_total":true`)
				}
				sharedErr := pulsecontract.ValidateRunningTotal(aggregation, temporality, running)
				rawErr := pulsecontract.ValidateDefinition(json.RawMessage(raw))
				if (sharedErr == nil) != (rawErr == nil) {
					t.Fatalf("aggregation=%s temporality=%s running=%t shared=%v raw=%v", aggregation, temporality, running, sharedErr, rawErr)
				}
				if sharedErr != nil && sharedErr.Error() != "running total requires SUM aggregation and OVER_TIME temporality" {
					t.Fatal(sharedErr)
				}
			}
		}
	}
	for _, replacement := range []string{``, `,"temporality":"TEMPORALITY_UNSPECIFIED"`} {
		raw := strings.ReplaceAll(definition, `,"temporality":"TEMPORALITY_OVER_TIME"`, replacement)
		raw = strings.ReplaceAll(raw, `"is_running_total":false`, `"is_running_total":true`)
		if err := pulsecontract.ValidateDefinition(json.RawMessage(raw)); err != nil {
			t.Fatalf("saved default temporality: %v", err)
		}
	}
}
