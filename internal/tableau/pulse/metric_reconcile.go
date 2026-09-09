package pulse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
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
			for _, filter := range filters {
				if entry, ok := filter.(map[string]any); ok {
					if values, ok := entry["categorical_values"].([]any); ok {
						sortJSONValues(values)
					}
				}
			}
			sortJSONValues(filters)
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
	sort.SliceStable(values, func(i, j int) bool {
		left, _ := json.Marshal(values[i])
		right, _ := json.Marshal(values[j])
		return bytes.Compare(left, right) < 0
	})
}

func statusCode(err error) int {
	var status interface{ HTTPStatus() int }
	if errors.As(err, &status) {
		return status.HTTPStatus()
	}
	return 0
}
