package value

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
)

func sortJSONValues(values []any) {
	slices.SortFunc(values, func(left, right any) int {
		leftJSON, _ := json.Marshal(left)
		rightJSON, _ := json.Marshal(right)
		return bytes.Compare(leftJSON, rightJSON)
	})
}

// NormalizePulseFilterArray reconciles categorical values with their legacy text
// representation and sorts the comparison data in place.
func NormalizePulseFilterArray(filters []any) error {
	for _, filter := range filters {
		entry, ok := filter.(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeFilterRepresentations(entry); err != nil {
			return err
		}
	}
	sortJSONValues(filters)
	return nil
}

// normalizeFilterRepresentations retains the complete typed population and
// checks its legacy mirror independently. The documented NULL_VALUE and native
// protobuf null are equivalent; null inclusion leaves that member out of the
// text mirror without removing it from the typed population.
func normalizeFilterRepresentations(entry map[string]any) error {
	rawCategorical, hasCategorical := entry["categorical_values"]
	rawText, hasText := entry["values"]
	if !hasCategorical && !hasText {
		return nil
	}

	var categorical, mirror []any
	if hasCategorical {
		includeNull, _ := entry["include_null"].(bool)
		var err error
		categorical, mirror, err = typedFilterValues(rawCategorical, includeNull)
		if err != nil {
			return err
		}
	}
	if hasText {
		text, err := textFilterValues(rawText)
		if err != nil {
			return err
		}
		sortJSONValues(text)
		if hasCategorical {
			sortJSONValues(mirror)
			mirrorJSON, _ := json.Marshal(mirror)
			textJSON, _ := json.Marshal(text)
			if !bytes.Equal(mirrorJSON, textJSON) {
				return errors.New("filter categorical_values and values conflict")
			}
		} else {
			categorical = text
		}
	}
	sortJSONValues(categorical)
	entry["categorical_values"] = categorical
	delete(entry, "values")
	return nil
}

func typedFilterValues(raw any, includeNull bool) ([]any, []any, error) {
	values, ok := raw.([]any)
	if !ok {
		return nil, nil, errors.New("filter categorical_values must be an array")
	}
	mirror := make([]any, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, nil, errors.New("filter categorical_values must contain typed objects")
		}
		nullValue, hasNull := item["null_value"]
		if len(item) == 1 && hasNull {
			if nullValue == "NULL_VALUE" {
				item["null_value"], nullValue = nil, nil
			}
			if includeNull && nullValue == nil {
				continue
			}
		}
		mirror = append(mirror, item)
	}
	return values, mirror, nil
}

func textFilterValues(raw any) ([]any, error) {
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("filter values must be an array")
	}
	converted := make([]any, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, errors.New("filter values must contain strings")
		}
		converted[i] = map[string]any{"string_value": text}
	}
	return converted, nil
}
