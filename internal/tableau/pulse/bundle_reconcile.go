package pulse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// VerifyBundleDefinition checks the shared definition once before recreating its
// metrics. Only 404 visibility delays are retried, within the normal poll bound.
func (c *Client) VerifyBundleDefinition(ctx context.Context, definition, datasource, site string, submitted json.RawMessage) error {
	if definition == "" || datasource == "" || (site != "" && site != c.session.SiteLUID()) {
		return errors.New("bundle definition ownership is not verified")
	}
	pollCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()
	for {
		saved, err := c.GetDefinition(pollCtx, definition)
		if err == nil {
			if saved.DatasourceLUID != datasource {
				return errors.New("bundle definition datasource ownership mismatch")
			}
			return compareBundleDefinition(saved, submitted)
		}
		if statusCode(err) != 404 {
			return err
		}
		if err := waitBundleReadback(pollCtx, c.pollInterval); err != nil {
			return err
		}
	}
}

// ReconcileBundleMetric verifies only metric state. The publishing action must
// first verify the shared definition using VerifyBundleDefinition.
func (c *Client) ReconcileBundleMetric(ctx context.Context, expected ExpectedMetric) (Reconciliation, error) {
	result := Reconciliation{Status: "pending_readback"}
	if expected.MetricLUID == "" || expected.DefinitionLUID == "" || expected.DatasourceLUID == "" || expected.Specification == nil {
		return result, errors.New("bundle metric verification requires authoritative identities and specification")
	}
	if expected.SiteLUID != "" && expected.SiteLUID != c.session.SiteLUID() {
		result.Status = "ownership_mismatch"
		return result, nil
	}
	pollCtx, cancel := context.WithTimeout(ctx, c.pollTimeout)
	defer cancel()
	for {
		result.Attempts++
		metric, err := c.GetMetric(pollCtx, expected.MetricLUID)
		if err == nil {
			result.Metric, result.TableauRequestID = metric, metric.TableauRequestID
			if metric.DefinitionLUID != expected.DefinitionLUID || (metric.SiteLUID != "" && metric.SiteLUID != c.session.SiteLUID()) {
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
			result.OwnershipVerified = true
			if !sameMetricSpecification(metric.Specification, expected.Specification) {
				result.Status = "specification_mismatch"
				return result, nil
			}
			result.SpecificationVerified, result.Status = true, "verified"
			return result, nil
		}
		if statusCode(err) != 404 {
			return result, err
		}
		if err := waitBundleReadback(pollCtx, c.pollInterval); err != nil {
			return result, err
		}
	}
}

func waitBundleReadback(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func compareBundleDefinition(saved Definition, submitted json.RawMessage) error {
	decode := func(raw json.RawMessage) (map[string]any, error) {
		var document map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&document); err != nil || document == nil {
			return nil, errors.New("bundle definition readback requires a configuration object")
		}
		return document, nil
	}
	requested, err := decode(submitted)
	if err != nil {
		return err
	}
	actual, err := decode(saved.Configuration)
	if err != nil {
		return err
	}
	actual["name"], actual["description"] = saved.Name, saved.Description
	// Provider identity, timestamps, and other response envelope fields are not
	// submitted business configuration. Compare every admitted submitted section.
	allowed := map[string]bool{"name": true, "description": true, "specification": true, "extension_options": true, "representation_options": true, "insights_options": true, "comparisons": true, "datasource_goals": true, "related_links": true, "certification": true}
	sections := make([]string, 0, len(requested))
	for section := range requested {
		sections = append(sections, section)
	}
	sort.Strings(sections)
	for _, section := range sections {
		wanted := requested[section]
		if !allowed[section] {
			return fmt.Errorf("unsupported submitted definition section %q", section)
		}
		observed, exists := actual[section]
		if !exists {
			return fmt.Errorf("saved definition omitted submitted section %q", section)
		}
		if err := normalizeBundleSection(section, wanted); err != nil {
			return fmt.Errorf("submitted definition configuration: %w", err)
		}
		if err := normalizeBundleSection(section, observed); err != nil {
			return fmt.Errorf("saved definition configuration: %w", err)
		}
		left, leftErr := json.Marshal(wanted)
		right, rightErr := json.Marshal(observed)
		if leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
			return fmt.Errorf("saved definition configuration mismatch in %q", section)
		}
	}
	return nil
}

// Defaults are narrowly drawn from the local Pulse definition API capture.
// Unknown nested business fields remain part of the comparison.
func normalizeBundleSection(section string, value any) error {
	object, ok := value.(map[string]any)
	if !ok {
		if section == "datasource_goals" {
			if goals, ok := value.([]any); ok {
				for _, goal := range goals {
					if object, ok := goal.(map[string]any); ok {
						normalizeBundleFilters(object["basic_specification"])
						normalizeBundleFilters(object["threshold_basic_specification"])
					}
				}
			}
		}
		return nil
	}
	setDefault := func(key string, value any) {
		if _, exists := object[key]; !exists {
			object[key] = value
		}
	}
	switch section {
	case "specification":
		setDefault("is_running_total", false)
		setDefault("temporality", "TEMPORALITY_OVER_TIME")
		if object["temporality"] == "TEMPORALITY_UNSPECIFIED" {
			object["temporality"] = "TEMPORALITY_OVER_TIME"
		}
		normalizeBundleFilters(object["basic_specification"])
	case "extension_options":
		setDefault("offset_from_today", json.Number("0"))
		setDefault("use_dynamic_offset", false)
	case "representation_options":
		setDefault("currency_code", "CURRENCY_CODE_USD")
	case "insights_options":
		if settings, ok := object["settings"].([]any); ok {
			for _, setting := range settings {
				if entry, ok := setting.(map[string]any); ok {
					if _, exists := entry["disabled"]; !exists {
						entry["disabled"] = false
					}
				}
			}
		}
	case "comparisons":
		if comparisons, ok := object["comparisons"].([]any); ok {
			for i, comparison := range comparisons {
				if err := normalizeBundleComparison(comparison, fmt.Sprintf("comparisons.comparisons[%d]", i)); err != nil {
					return err
				}
			}
		}
		if nested, exists := object["nestedComparison"]; exists {
			return normalizeBundleComparison(nested, "comparisons.nestedComparison")
		}
	case "certification":
		delete(object, "modified_at")
		delete(object, "modified_by")
	}
	return nil
}

func normalizeBundleComparison(value any, path string) error {
	comparison, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if index, exists := comparison["index"]; exists {
		var decimal string
		switch index := index.(type) {
		case json.Number:
			decimal = index.String()
		case string:
			decimal = index
		default:
			return fmt.Errorf("%s.index must be an exact signed 64-bit integer", path)
		}
		decimal = strings.TrimSpace(decimal)
		parsed, err := strconv.ParseInt(decimal, 10, 64)
		if err != nil || !json.Valid([]byte(decimal)) {
			return fmt.Errorf("%s.index must be an exact signed 64-bit integer", path)
		}
		// Canonicalize only the decoded comparison copy, retaining integer precision
		// and the original submitted payload and all unknown business fields.
		comparison["index"] = strconv.FormatInt(parsed, 10)
	}
	if nested, exists := comparison["nestedComparison"]; exists {
		return normalizeBundleComparison(nested, path+".nestedComparison")
	}
	return nil
}

func normalizeBundleFilters(value any) {
	if basic, ok := value.(map[string]any); ok {
		if filters, ok := basic["filters"].([]any); ok {
			for _, filter := range filters {
				if entry, ok := filter.(map[string]any); ok {
					if values, ok := entry["categorical_values"].([]any); ok {
						sortJSONValues(values)
					}
				}
			}
			sortJSONValues(filters)
		}
	}
}
