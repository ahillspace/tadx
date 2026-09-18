package pulse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// ReconcileMetric reads saved state by authoritative identity. List visibility is
// neither necessary nor sufficient evidence that a requested variant was saved.
func (c *Client) ReconcileMetric(ctx context.Context, expected ExpectedMetric) (Reconciliation, error) {
	if expected.MetricLUID == "" || expected.DefinitionLUID == "" || expected.DatasourceLUID == "" || expected.Specification == nil {
		return Reconciliation{}, errors.New("Pulse metric reconciliation requires metric, definition, datasource LUIDs and the requested specification")
	}
	result := Reconciliation{Status: "pending_readback"}
	if expected.SiteLUID != "" && expected.SiteLUID != c.session.SiteLUID() {
		result.Status = "ownership_mismatch"
		return result, nil
	}
	pollCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()
	for {
		if err := pollCtx.Err(); err != nil {
			return result, err
		}
		result.Attempts++
		if result.Metric.LUID == "" {
			metric, err := c.GetMetric(pollCtx, expected.MetricLUID)
			if err != nil {
				if pollCtx.Err() != nil {
					return result, pollCtx.Err()
				}
				if statusCode(err) != 404 {
					return result, err
				}
			} else {
				result.TableauRequestID = metric.TableauRequestID
				if metric.LUID != expected.MetricLUID || metric.DefinitionLUID != expected.DefinitionLUID || (metric.SiteLUID != "" && metric.SiteLUID != c.session.SiteLUID()) {
					result.Status = "ownership_mismatch"
					return result, nil
				}
				if raw, exists := metric.Specification["datasource"]; exists {
					datasource, ok := raw.(map[string]any)
					if !ok || datasource["id"] != expected.DatasourceLUID {
						result.Status = "ownership_mismatch"
						return result, nil
					}
				}
				if !sameMetricSpecification(metric.Specification, expected.Specification) {
					result.Status = "specification_mismatch"
					return result, nil
				}
				result.Metric = metric
				result.SpecificationVerified = true
			}
		}
		if result.Metric.LUID != "" {
			definition, err := c.GetDefinition(pollCtx, expected.DefinitionLUID)
			if err != nil {
				if pollCtx.Err() != nil {
					return result, pollCtx.Err()
				}
				if statusCode(err) != 404 {
					return result, err
				}
			} else {
				result.TableauRequestID = definition.TableauRequestID
				if definition.LUID != expected.DefinitionLUID || definition.DatasourceLUID != expected.DatasourceLUID {
					result.Status = "ownership_mismatch"
					return result, nil
				}
				result.Definition = definition
				result.OwnershipVerified = true
				result.Status = "verified"
				return result, nil
			}
		}
		timer := time.NewTimer(c.pollInterval)
		select {
		case <-pollCtx.Done():
			timer.Stop()
			return result, pollCtx.Err()
		case <-timer.C:
		}
	}
}

// Compare saved configuration losslessly, allowing only unordered dimensional
// filters/values and the datasource reference that the server adds to metrics.
func sameMetricSpecification(saved, requested map[string]any) bool {
	normalize := func(spec map[string]any) ([]byte, error) {
		if spec == nil {
			return nil, errors.New("missing specification")
		}
		data, err := json.Marshal(spec)
		if err != nil {
			return nil, err
		}
		var copy map[string]any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&copy); err != nil {
			return nil, err
		}
		delete(copy, "datasource")
		if filters, ok := copy["filters"].([]any); ok {
			if err := normalizeFilterArray(filters); err != nil {
				return nil, err
			}
		}
		return json.Marshal(copy)
	}
	savedJSON, err := normalize(saved)
	if err != nil {
		return false
	}
	requestedJSON, err := normalize(requested)
	return err == nil && bytes.Equal(savedJSON, requestedJSON)
}

func sortJSONValues(values []any) {
	slices.SortFunc(values, func(left, right any) int {
		leftJSON, _ := json.Marshal(left)
		rightJSON, _ := json.Marshal(right)
		return bytes.Compare(leftJSON, rightJSON)
	})
}

func normalizeFilterArray(filters []any) error {
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

// normalizeFilterRepresentations makes the provider's typed categorical values
// and the portable text values comparable without ignoring conflicting fields.
func normalizeFilterRepresentations(entry map[string]any) error {
	rawCategorical, hasCategorical := entry["categorical_values"]
	rawText, hasText := entry["values"]
	if !hasCategorical && !hasText {
		return nil
	}

	var categorical []any
	if hasCategorical {
		var err error
		categorical, err = typedFilterValues(rawCategorical)
		if err != nil {
			return err
		}
	}
	if hasText {
		text, err := textFilterValues(rawText)
		if err != nil {
			return err
		}
		if hasCategorical {
			sortJSONValues(categorical)
			sortJSONValues(text)
			categoricalJSON, _ := json.Marshal(categorical)
			textJSON, _ := json.Marshal(text)
			if !bytes.Equal(categoricalJSON, textJSON) {
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

func typedFilterValues(raw any) ([]any, error) {
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("filter categorical_values must be an array")
	}
	for _, value := range values {
		if _, ok := value.(map[string]any); !ok {
			return nil, errors.New("filter categorical_values must contain typed objects")
		}
	}
	return values, nil
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

func statusCode(err error) int {
	var status interface{ HTTPStatus() int }
	if errors.As(err, &status) {
		return status.HTTPStatus()
	}
	return 0
}
