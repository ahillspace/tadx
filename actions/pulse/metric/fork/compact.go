package fork

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// CompactPlan describes the resulting saved population, not just CLI edits.
type CompactPlan struct {
	Mode               string            `json:"mode"`
	Operation          string            `json:"operation"`
	Environment        string            `json:"environment"`
	Site               string            `json:"site"`
	SourceMetricLUID   string            `json:"source_metric_luid"`
	DefinitionLUID     string            `json:"definition_luid"`
	DatasourceLUID     string            `json:"datasource_luid"`
	Period             PeriodSummary     `json:"period"`
	Population         PopulationSummary `json:"population"`
	AdditionalSettings int               `json:"additional_settings_in_full,omitempty"`
	RequiresFull       bool              `json:"requires_full"`
	ReviewComplete     bool              `json:"review_complete"`
}

type PeriodSummary struct {
	Source        string `json:"source"`
	Granularity   string `json:"granularity"`
	Range         string `json:"range"`
	Periods       string `json:"periods,omitempty"`
	PeriodUnit    string `json:"period_unit,omitempty"`
	CurrentPeriod string `json:"current_period,omitempty"`
	Unrepresented int    `json:"unrepresented_settings,omitempty"`
}

type PopulationSummary struct {
	Scope          string             `json:"scope"`
	Combination    string             `json:"combination"`
	Filters        []PopulationFilter `json:"filters"`
	FiltersOmitted int                `json:"filters_omitted,omitempty"`
	Unrepresented  int                `json:"unrepresented_settings,omitempty"`
}

type PopulationFilter struct {
	Field           string            `json:"field"`
	Source          string            `json:"source"`
	Operator        string            `json:"operator"`
	Values          []PopulationValue `json:"values"`
	Nulls           string            `json:"nulls"`
	ValuesOmitted   int               `json:"values_omitted,omitempty"`
	TruncatedValues int               `json:"truncated_values,omitempty"`
	Unrepresented   int               `json:"unrepresented_settings,omitempty"`
}

type PopulationValue struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func compactPlan(plan Plan) CompactPlan {
	out := CompactPlan{Mode: plan.Mode, Operation: plan.Operation, Environment: plan.Environment, Site: plan.Site, SourceMetricLUID: plan.SourceMetricLUID, DefinitionLUID: plan.DefinitionLUID, DatasourceLUID: plan.DatasourceLUID}
	out.Period = periodSummary(plan)
	out.Population = populationSummary(plan)
	for key := range plan.Specification {
		if key != "measurement_period" && key != "filters" && key != "datasource" {
			out.AdditionalSettings++
		}
	}
	out.RequiresFull = out.Period.Unrepresented > 0 || out.Population.Unrepresented > 0 || out.Population.FiltersOmitted > 0 || out.AdditionalSettings > 0
	for _, filter := range out.Population.Filters {
		out.RequiresFull = out.RequiresFull || filter.ValuesOmitted > 0 || filter.TruncatedValues > 0 || filter.Unrepresented > 0
	}
	out.ReviewComplete = !out.RequiresFull
	return out
}

func periodSummary(plan Plan) PeriodSummary {
	out := PeriodSummary{Source: "INHERITED"}
	if plan.Timeframe != "" {
		out.Source = "CHANGED"
	}
	period, ok := plan.Specification["measurement_period"].(map[string]any)
	if !ok {
		out.Unrepresented = 1
		return out
	}
	out.Granularity = strings.TrimPrefix(stringValue(period["granularity"]), "GRANULARITY_BY_")
	out.Range = strings.TrimPrefix(stringValue(period["range"]), "RANGE_")
	if !slices.Contains([]string{"DAY", "WEEK", "MONTH", "QUARTER", "YEAR"}, out.Granularity) || !slices.Contains([]string{"CURRENT_PARTIAL", "LAST_COMPLETE", "BY_CONFIG"}, out.Range) {
		out.Unrepresented++
	}
	for key := range period {
		if key != "granularity" && key != "range" && key != "last_x_period" {
			out.Unrepresented++
		}
	}
	if raw, exists := period["last_x_period"]; exists {
		rolling, ok := raw.(map[string]any)
		if !ok {
			out.Unrepresented++
			return out
		}
		out.Periods = fmt.Sprint(rolling["period"])
		if count, err := strconv.ParseInt(out.Periods, 10, 64); err != nil || count < 1 {
			out.Unrepresented++
		}
		out.PeriodUnit = strings.TrimPrefix(stringValue(rolling["period_type"]), "GRANULARITY_BY_")
		out.CurrentPeriod = nullSummary(rolling["include_current_period"])
		if rolling["period"] == nil || out.PeriodUnit == "" || out.CurrentPeriod == "UNSPECIFIED" {
			out.Unrepresented++
		}
		for key := range rolling {
			if key != "period" && key != "period_type" && key != "include_current_period" {
				out.Unrepresented++
			}
		}
	} else if out.Range == "BY_CONFIG" {
		out.Unrepresented++
	}
	return out
}

func populationSummary(plan Plan) PopulationSummary {
	out := PopulationSummary{Scope: "ALL_ROWS", Combination: "AND", Filters: []PopulationFilter{}}
	raw, exists := plan.Specification["filters"]
	filters, ok := raw.([]any)
	if !exists || !ok {
		out.Scope = "UNSPECIFIED"
		out.Unrepresented = 1
	}
	metricFilterCount := len(filters)
	filters = append(append([]any{}, filters...), plan.DefinitionFilters...)
	if !plan.DefinitionFiltersKnown {
		out.Unrepresented++
		out.Scope = "UNSPECIFIED"
	}
	if len(filters) > 0 {
		out.Scope = "FILTERED"
	}
	out.FiltersOmitted = max(0, len(filters)-25)
	changed := map[string]bool{}
	for _, filter := range plan.Filters {
		changed[filter.Field] = true
	}
	for index, raw := range filters[:min(25, len(filters))] {
		filter, ok := raw.(map[string]any)
		if !ok {
			out.Unrepresented++
			continue
		}
		item := PopulationFilter{Field: stringValue(filter["field"]), Source: "INHERITED", Operator: strings.TrimPrefix(stringValue(filter["operator"]), "OPERATOR_"), Nulls: nullSummary(filter["include_null"]), Values: []PopulationValue{}}
		if changed[item.Field] {
			item.Source = "CHANGED"
		}
		if index >= metricFilterCount {
			item.Source = "DEFINITION_FIXED"
		}
		if item.Field == "" || (item.Operator != "EQUAL" && item.Operator != "NOT_EQUAL") || item.Nulls == "UNSPECIFIED" {
			item.Unrepresented++
		}
		for key := range filter {
			if key != "field" && key != "operator" && key != "categorical_values" && key != "include_null" {
				item.Unrepresented++
			}
		}
		values, ok := filter["categorical_values"].([]any)
		if !ok {
			item.Unrepresented++
		}
		item.ValuesOmitted = max(0, len(values)-20)
		for _, raw := range values[:min(20, len(values))] {
			value, ok := raw.(map[string]any)
			if !ok || len(value) != 1 {
				item.Unrepresented++
				continue
			}
			rendered := PopulationValue{}
			switch {
			case value["string_value"] != nil:
				if _, ok := value["string_value"].(string); !ok {
					item.Unrepresented++
					continue
				}
				rendered.Kind = "STRING"
				rendered.Value = fmt.Sprint(value["string_value"])
			case value["bool_value"] != nil:
				if _, ok := value["bool_value"].(bool); !ok {
					item.Unrepresented++
					continue
				}
				rendered.Kind = "BOOLEAN"
				rendered.Value = fmt.Sprint(value["bool_value"])
			default:
				if _, ok := value["null_value"]; ok {
					rendered.Kind = "NULL"
					rendered.Value = "null"
				} else {
					item.Unrepresented++
					continue
				}
			}
			runes := []rune(rendered.Value)
			if len(runes) > 256 {
				rendered.Value = string(runes[:256]) + " [truncated; --full]"
				item.TruncatedValues++
			}
			item.Values = append(item.Values, rendered)
		}
		out.Filters = append(out.Filters, item)
	}
	return out
}

func nullSummary(value any) string {
	if flag, ok := value.(bool); ok {
		if flag {
			return "INCLUDED"
		}
		return "EXCLUDED"
	}
	return "UNSPECIFIED"
}
