package value_test

import (
	"encoding/json"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestPulseDefinitionCreateRequestWireBytes(t *testing.T) {
	falseValue := false
	textValue := "West"
	request := value.PulseDefinitionCreateRequest{
		Name: "Revenue", Description: "Sales",
		Specification: value.PulseSpecification{
			Datasource: value.PulseDatasource{ID: "datasource-1"},
			BasicSpecification: value.PulseBasicSpecification{
				Measure:       value.PulseMeasure{Field: "Sales", Aggregation: "AGGREGATION_SUM"},
				TimeDimension: value.PulseTimeDimension{Field: "Order Date"},
				Filters:       []value.PulseFilter{{Field: "Region", Operator: "IN", CategoricalValues: []value.PulseCategoricalValue{{BoolValue: &falseValue}, {StringValue: &textValue}}, IncludeNull: false}},
			},
			RunningTotal: false, Temporality: "TEMPORALITY_OVER_TIME",
		},
		ExtensionOptions:      value.PulseExtensionOptions{AllowedDimensions: []string{}, AllowedGranularities: nil, OffsetFromToday: 0, UseDynamicOffset: false},
		RepresentationOptions: value.PulseRepresentationOptions{Type: "NUMBER_FORMAT_TYPE_NUMBER", SentimentType: "SENTIMENT_TYPE_NONE"},
		InsightsOptions:       value.PulseInsightsOptions{ShowInsights: true, Settings: []value.PulseInsightSetting{{Type: "INSIGHT_TYPE_TOP_DRIVERS", Disabled: false}}},
		Comparisons:           value.PulseComparisons{Comparisons: []value.PulseComparison{{CompareConfig: value.PulseCompareConfig{Comparison: "TIME_COMPARISON_PREVIOUS_PERIOD"}, Index: 0}}},
		DatasourceGoals:       []map[string]any{}, RelatedLinks: nil, Certification: value.PulseCertification{IsCertified: false},
	}
	got, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"Revenue","description":"Sales","specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"measure":{"field":"Sales","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"},"filters":[{"field":"Region","operator":"IN","categorical_values":[{"bool_value":false},{"string_value":"West"}],"include_null":false}]},"is_running_total":false,"temporality":"TEMPORALITY_OVER_TIME"},"extension_options":{"allowed_dimensions":[],"allowed_granularities":null,"offset_from_today":0,"use_dynamic_offset":false},"representation_options":{"type":"NUMBER_FORMAT_TYPE_NUMBER","sentiment_type":"SENTIMENT_TYPE_NONE"},"insights_options":{"show_insights":true,"settings":[{"type":"INSIGHT_TYPE_TOP_DRIVERS","disabled":false}]},"comparisons":{"comparisons":[{"compare_config":{"comparison":"TIME_COMPARISON_PREVIOUS_PERIOD"},"index":0}]},"datasource_goals":[],"related_links":null,"certification":{"is_certified":false}}`
	if string(got) != want {
		t.Fatalf("request bytes changed\nwant: %s\ngot:  %s", want, got)
	}
}
