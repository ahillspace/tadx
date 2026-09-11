// Package pulsecontract validates known saved Pulse payload contracts without
// projecting them into the smaller flag-based creation model.
package pulsecontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"
)

type check func(json.RawMessage, string) error
type fields map[string]check

var granularity = enum("GRANULARITY_UNSPECIFIED", "GRANULARITY_BY_DAY", "GRANULARITY_BY_WEEK", "GRANULARITY_BY_MONTH", "GRANULARITY_BY_QUARTER", "GRANULARITY_BY_YEAR", "GRANULARITY_BY_FISCAL_QUARTER", "GRANULARITY_BY_FISCAL_YEAR")
var aggregation = enum("AGGREGATION_UNSPECIFIED", "AGGREGATION_SUM", "AGGREGATION_AVERAGE", "AGGREGATION_MEDIAN", "AGGREGATION_MIN", "AGGREGATION_MAX", "AGGREGATION_COUNT", "AGGREGATION_COUNT_DISTINCT", "AGGREGATION_USER")
var timeDimension = object(fields{"field": nonblank}, "field")
var measure = object(fields{"field": nonblank, "aggregation": aggregation}, "field", "aggregation")
var datasource = object(fields{
	"id":               nonblank,
	"id_type":          enum("DATASOURCE_ID_TYPE_UNSPECIFIED", "DATASOURCE_ID_TYPE_LUID", "DATASOURCE_ID_TYPE_WORKBOOK_DATASOURCE"),
	"vizql_session_id": textValue,
}, "id")

// ValidateDefinition checks a raw definition create document locally.
// Unknown properties remain valid and the caller's bytes are never changed.
func ValidateDefinition(raw json.RawMessage) error {
	return object(fields{
		"name": boundedString(255, true), "description": boundedString(1024, false),
		"specification": definitionSpecification,
		"extension_options": object(fields{
			"allowed_dimensions": array(nonblank), "allowed_granularities": array(granularity),
			"offset_from_today": int32Value, "use_dynamic_offset": boolean,
			"correlation_candidate_definition_ids": array(textValue),
			"custom_calendar":                      object(fields{"day_of_year_field": textValue, "month_name_field": textValue, "month_of_year_field": textValue, "quarter_field": textValue, "reference_date_field": textValue, "week_of_year_field": textValue, "year_field": textValue}),
		}),
		"representation_options": representation,
		"insights_options":       object(fields{"show_insights": boolean, "settings": array(insightSetting), "nestedInsightSetting": insightSetting}),
		"comparisons":            object(fields{"comparisons": array(comparison), "nestedComparison": comparison}),
		"datasource_goals":       array(goal),
		"related_links":          array(object(fields{"link_name": textValue, "link_url": textValue})),
		"certification":          object(fields{"is_certified": boolean, "modified_at": textValue, "modified_by": textValue}),
	}, "name", "specification")(raw, "definition")
}

// ValidateMetric checks a complete saved metric specification locally.
// Saved period extensions are preserved rather than rebuilt from CLI flags.
func ValidateMetric(raw json.RawMessage) error {
	return object(fields{
		"datasource": datasource, "measurement_period": measurementPeriod,
		"filters": array(filter), "comparison": compareConfig,
	}, "measurement_period", "filters")(raw, "metric")
}

func definitionSpecification(raw json.RawMessage, path string) error {
	if err := object(fields{
		"datasource": datasource, "basic_specification": basicSpecification,
		"is_running_total":             boolean,
		"temporality":                  enum("TEMPORALITY_UNSPECIFIED", "TEMPORALITY_OVER_TIME", "TEMPORALITY_LATEST_POINT_IN_TIME"),
		"viz_state_specification":      vizState,
		"abstract_query_specification": object(fields{"abstract_query_string": textValue}),
	}, "datasource", "basic_specification")(raw, path); err != nil {
		return err
	}
	var spec map[string]json.RawMessage
	_ = json.Unmarshal(raw, &spec)
	var running bool
	_ = json.Unmarshal(spec["is_running_total"], &running)
	if running {
		var temporality string
		_ = json.Unmarshal(spec["temporality"], &temporality)
		if temporality == "" || temporality == "TEMPORALITY_UNSPECIFIED" {
			temporality = "TEMPORALITY_OVER_TIME"
		}
		var basic struct{ Measure struct{ Aggregation string } }
		_ = json.Unmarshal(spec["basic_specification"], &basic)
		if err := ValidateRunningTotal(basic.Measure.Aggregation, temporality, running); err != nil {
			return fmt.Errorf("%s.is_running_total: %w", path, err)
		}
	}
	return nil
}

// ValidateRunningTotal checks the invariant shared by raw and flag-built requests.
// Callers resolve defaults before passing provider enum values to this function.
func ValidateRunningTotal(aggregation, temporality string, running bool) error {
	if running && (aggregation != "AGGREGATION_SUM" || temporality != "TEMPORALITY_OVER_TIME") {
		return errors.New("running total requires SUM aggregation and OVER_TIME temporality")
	}
	return nil
}

func basicSpecification(raw json.RawMessage, path string) error {
	return object(fields{"measure": measure, "time_dimension": timeDimension, "filters": array(filter)}, "measure", "time_dimension", "filters")(raw, path)
}

func filter(raw json.RawMessage, path string) error {
	return object(fields{
		"field": nonblank, "operator": enum("OPERATOR_UNSPECIFIED", "OPERATOR_EQUAL", "OPERATOR_NOT_EQUAL"),
		"categorical_values": array(categoricalValue), "include_null": boolean, "values": array(textValue),
	}, "field", "operator", "categorical_values")(raw, path)
}

func categoricalValue(raw json.RawMessage, path string) error {
	if err := object(fields{"string_value": textValue, "bool_value": boolean, "null_value": enum("NULL_VALUE")})(raw, path); err != nil {
		return err
	}
	var values map[string]json.RawMessage
	_ = json.Unmarshal(raw, &values)
	count := 0
	for _, key := range []string{"string_value", "bool_value", "null_value"} {
		if _, ok := values[key]; ok {
			count++
		}
	}
	if count > 1 || len(values) == 0 {
		return fmt.Errorf("%s requires one categorical value", path)
	}
	return nil
}

func measurementPeriod(raw json.RawMessage, path string) error {
	if err := object(fields{
		"granularity":   granularity,
		"range":         enum("RANGE_UNSPECIFIED", "RANGE_CURRENT_PARTIAL", "RANGE_LAST_COMPLETE", "RANGE_BY_CONFIG", "RANGE_LAST_N"),
		"last_x_period": object(fields{"period": positiveInteger, "period_type": granularity, "include_current_period": boolean}, "period", "period_type"),
		"last_n":        positiveInteger, "offset": integer,
	}, "granularity", "range")(raw, path); err != nil {
		return err
	}
	var period map[string]json.RawMessage
	_ = json.Unmarshal(raw, &period)
	var rangeName string
	_ = json.Unmarshal(period["range"], &rangeName)
	if rangeName == "RANGE_LAST_N" && len(period["last_n"]) == 0 {
		return fmt.Errorf("%s.last_n is required for RANGE_LAST_N", path)
	}
	// RANGE_BY_CONFIG can carry provider configurations beyond last_x_period.
	// Preserve opaque extensions without claiming to validate their semantics.
	if rangeName == "RANGE_BY_CONFIG" {
		for name, configuration := range period {
			switch name {
			case "granularity", "range", "offset", "last_n":
				continue
			}
			if !bytes.Equal(bytes.TrimSpace(configuration), []byte("null")) {
				return nil
			}
		}
		return fmt.Errorf("%s requires a saved period configuration", path)
	}
	return nil
}

func compareConfig(raw json.RawMessage, path string) error {
	return object(fields{
		"comparison":                 enum("TIME_COMPARISON_UNSPECIFIED", "TIME_COMPARISON_NONE", "TIME_COMPARISON_PREVIOUS_PERIOD", "TIME_COMPARISON_YEAR_AGO_PERIOD", "TIME_COMPARISON_FISCAL_YEAR_AGO_PERIOD"),
		"comparison_period_override": array(object(fields{"comparison_time": timeDimension, "reference_time": timeDimension, "granularity": granularity})),
	})(raw, path)
}

func comparison(raw json.RawMessage, path string) error {
	return object(fields{"compare_config": compareConfig, "index": int64Value, "nestedComparison": comparison})(raw, path)
}

func goal(raw json.RawMessage, path string) error {
	// Goal specifications can inherit definition fields. Check present fields
	// without imposing the definition's required-field contract on those goals.
	goalBasic := object(fields{
		"measure":        object(fields{"field": nonblank, "aggregation": aggregation}),
		"time_dimension": object(fields{"field": nonblank}), "filters": array(filter),
	})
	return object(fields{
		"name": textValue, "minimum_granularity": granularity,
		"basic_specification": goalBasic, "threshold_basic_specification": goalBasic,
		"viz_state_specification": vizState, "threshold_viz_state_specification": vizState,
		"benchmark_sentiment_type": enum("BENCHMARK_SENTIMENT_TYPE_UNSPECIFIED", "BENCHMARK_SENTIMENT_TYPE_NONE", "BENCHMARK_SENTIMENT_TYPE_ABOVE_THRESHOLD_IS_UNFAVORABLE", "BENCHMARK_SENTIMENT_TYPE_BELOW_THRESHOLD_IS_UNFAVORABLE"),
	})(raw, path)
}

func vizState(raw json.RawMessage, path string) error {
	return object(fields{"viz_state_string": textValue})(raw, path)
}

func insightSetting(raw json.RawMessage, path string) error {
	return object(fields{"type": textValue, "disabled": boolean, "benchmark_breakdown_options": object(fields{"limit": int32Value}), "contributors_options": object(fields{"limit": int32Value})})(raw, path)
}

func representation(raw json.RawMessage, path string) error {
	numberUnits := object(fields{"plural_noun": textValue, "singular_noun": textValue})
	entityNames := object(fields{"entity_name_plural": textValue, "entity_name_singular": textValue})
	idField := object(fields{"identifier_col": textValue, "identifier_label": textValue})
	nameField := object(fields{"name_col": textValue})
	return object(fields{
		"type":           enum("NUMBER_FORMAT_TYPE_UNSPECIFIED", "NUMBER_FORMAT_TYPE_NUMBER", "NUMBER_FORMAT_TYPE_CURRENCY", "NUMBER_FORMAT_TYPE_PERCENT"),
		"sentiment_type": enum("SENTIMENT_TYPE_UNSPECIFIED", "SENTIMENT_TYPE_NONE", "SENTIMENT_TYPE_UP_IS_GOOD", "SENTIMENT_TYPE_DOWN_IS_GOOD"),
		"currency_code":  textValue, "decimal_places": integer, "positive_only": boolean,
		"number_units": numberUnits, "nestedNumberUnits": numberUnits,
		"row_level_entity_names": entityNames, "nestedRowLevelEntityNames": entityNames,
		"row_level_id_field": idField, "nestedRowLevelIDField": idField,
		"row_level_name_field": nameField, "nestedRowLevelNameField": nameField,
	})(raw, path)
}

func object(schema fields, required ...string) check {
	return func(raw json.RawMessage, path string) error {
		var value map[string]json.RawMessage
		if json.Unmarshal(raw, &value) != nil || value == nil {
			return fmt.Errorf("%s must be a JSON object", path)
		}
		for _, name := range required {
			if _, exists := value[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		keys := make([]string, 0, len(value))
		for name := range value {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			if validate, known := schema[name]; known {
				if err := validate(value[name], path+"."+name); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func array(element check) check {
	return func(raw json.RawMessage, path string) error {
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return fmt.Errorf("%s must be a JSON array", path)
		}
		for i, value := range values {
			if err := element(value, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	}
}

func textValue(raw json.RawMessage, path string) error {
	var value string
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &value) != nil {
		return fmt.Errorf("%s must be a JSON string", path)
	}
	return nil
}

func nonblank(raw json.RawMessage, path string) error { return boundedString(0, true)(raw, path) }

func boundedString(limit int, required bool) check {
	return func(raw json.RawMessage, path string) error {
		if err := textValue(raw, path); err != nil {
			return err
		}
		var value string
		_ = json.Unmarshal(raw, &value)
		if required && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be blank", path)
		}
		if limit > 0 && utf8.RuneCountInString(value) > limit {
			return fmt.Errorf("%s exceeds the %d-character limit", path, limit)
		}
		return nil
	}
}

func boolean(raw json.RawMessage, path string) error {
	var value bool
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &value) != nil {
		return fmt.Errorf("%s must be a JSON boolean", path)
	}
	return nil
}

func enum(allowed ...string) check {
	return func(raw json.RawMessage, path string) error {
		if err := textValue(raw, path); err != nil {
			return err
		}
		var value string
		_ = json.Unmarshal(raw, &value)
		for _, candidate := range allowed {
			if value == candidate {
				return nil
			}
		}
		return fmt.Errorf("%s contains an unsupported enum value", path)
	}
}

func integer(raw json.RawMessage, path string) error {
	value := strings.TrimSpace(string(raw))
	if !json.Valid(raw) || value == "" || strings.ContainsAny(value, ".eE\"") {
		return fmt.Errorf("%s must be a JSON integer", path)
	}
	if _, ok := new(big.Int).SetString(value, 10); !ok {
		return fmt.Errorf("%s must be a JSON integer", path)
	}
	return nil
}

func positiveInteger(raw json.RawMessage, path string) error {
	if err := integer(raw, path); err != nil {
		return err
	}
	value, _ := new(big.Int).SetString(strings.TrimSpace(string(raw)), 10)
	if value.Sign() <= 0 {
		return fmt.Errorf("%s must be a positive integer", path)
	}
	return nil
}

func int32Value(raw json.RawMessage, path string) error {
	if err := integer(raw, path); err != nil {
		return err
	}
	value, _ := new(big.Int).SetString(strings.TrimSpace(string(raw)), 10)
	if !value.IsInt64() || value.Int64() < -2147483648 || value.Int64() > 2147483647 {
		return fmt.Errorf("%s must fit a signed 32-bit integer", path)
	}
	return nil
}

func int64Value(raw json.RawMessage, path string) error {
	// Protobuf JSON int64 values can be strings; accept either exact encoding.
	var value string
	if json.Unmarshal(raw, &value) == nil && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = json.RawMessage(value)
	}
	if err := integer(raw, path); err != nil {
		return err
	}
	parsed, _ := new(big.Int).SetString(strings.TrimSpace(string(raw)), 10)
	if !parsed.IsInt64() {
		return fmt.Errorf("%s must fit a signed 64-bit integer", path)
	}
	return nil
}
