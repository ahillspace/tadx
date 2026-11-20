package definition

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/value"
)

func createFixedFilters(selections []CreateFilterSelection) ([]CreateFilter, error) {
	filters := make([]CreateFilter, len(selections))
	fields := make([]string, len(selections))
	for i, selection := range selections {
		field := strings.TrimSpace(selection.Field)
		if field == "" || len(selection.Values) == 0 {
			return nil, errors.New("every fixed filter requires a field and at least one value")
		}
		operator := "OPERATOR_EQUAL"
		if selection.Exclude {
			operator = "OPERATOR_NOT_EQUAL"
		}
		filter := CreateFilter{Field: field, Operator: operator, CategoricalValues: make([]CreateCategoricalValue, len(selection.Values))}
		for j, value := range selection.Values {
			if value == "" || utf8.RuneCountInString(value) > 256 {
				return nil, fmt.Errorf("fixed filter field %q requires values of 1 through 256 characters", field)
			}
			filter.CategoricalValues[j].StringValue = new(value)
		}
		filters[i], fields[i] = filter, field
	}
	return resolveFixedFilters(filters, fields)
}

// Resolve aliases before merging so a caption and raw ID cannot produce two
// filters on the same field. Retain literal values, including whitespace.
func resolveFixedFilters(filters []CreateFilter, fields []string) ([]CreateFilter, error) {
	if len(fields) != len(filters) {
		return nil, errors.New("fixed filter field resolution returned incomplete identities")
	}
	byField := make(map[string]CreateFilter, len(filters))
	for i, filter := range filters {
		field := fields[i]
		merged, exists := byField[field]
		if exists && merged.Operator != filter.Operator {
			return nil, fmt.Errorf("field %q has conflicting include and exclude filters", field)
		}
		merged.Field, merged.Operator = field, filter.Operator
		merged.CategoricalValues = append(merged.CategoricalValues, filter.CategoricalValues...)
		byField[field] = merged
	}
	result := make([]CreateFilter, 0, len(byField))
	for _, field := range slices.Sorted(maps.Keys(byField)) {
		filter := byField[field]
		values := filter.CategoricalValues
		slices.SortFunc(values, func(left, right CreateCategoricalValue) int {
			return cmp.Compare(*left.StringValue, *right.StringValue)
		})
		filter.CategoricalValues = slices.CompactFunc(values, func(left, right CreateCategoricalValue) bool { return *left.StringValue == *right.StringValue })
		if len(filter.CategoricalValues) > 10000 {
			return nil, fmt.Errorf("fixed filter field %q exceeds 10000 values", field)
		}
		result = append(result, filter)
	}
	return result, nil
}

func equivalentFixedFilters(saved []any, requested []CreateFilter) bool {
	if len(saved) != len(requested) {
		return false
	}
	if len(saved) == 0 {
		return true
	}
	savedJSON, err := canonicalDefinitionFilters(saved)
	if err != nil {
		return false
	}
	requestedJSON, err := canonicalDefinitionFilters(requested)
	return err == nil && bytes.Equal(savedJSON, requestedJSON)
}

func canonicalDefinitionFilters(filters any) ([]byte, error) {
	data, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}
	// Normalize an owned copy and preserve values in any unknown settings.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var comparison []any
	if err := decoder.Decode(&comparison); err != nil {
		return nil, err
	}
	for _, raw := range comparison {
		if filter, ok := raw.(map[string]any); ok && filter["include_null"] == false {
			delete(filter, "include_null")
		}
	}
	if err := value.NormalizePulseFilterArray(comparison); err != nil {
		return nil, err
	}
	return json.Marshal(comparison)
}
