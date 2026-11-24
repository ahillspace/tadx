package datasource

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

// TDSPrerequisites reports only locally observed negotiation. A successful
// native datasource GET or TDS read cannot qualify a later mutation.
func (c *Client) TDSPrerequisites(operation string) value.TDSEligibility {
	e := value.TDSEligibility{Operation: operation, Status: "unknown", Reason: "operation_not_observed", RESTStatus: "unknown", Permission: "unknown", Entitlement: "unknown"}
	if operation == "datasource.checkout" {
		e.SourceType, e.OpenCopyQuota = "unknown", "unknown"
	}
	if c == nil || c.transport == nil {
		return e
	}
	version := c.transport.APIVersion()
	supported, err := tableau.APIVersionAtLeast(version, 3, 30)
	if err != nil {
		e.Reason = "rest_version_unknown"
		return e
	}
	e.RESTVersion = version
	if !supported {
		e.RESTStatus, e.Status, e.Reason = "unsupported", "unsupported", "rest_version_too_low"
	} else {
		e.RESTStatus = "supported"
	}
	return e
}

func (c *Client) qualifyTDSReceipt(operation string, receipt *value.TDSReceipt, err error) {
	receipt.Eligibility = c.observedTDS(operation, receipt.Status == "acknowledged", receipt.Failure, err)
	// Checkout has explicit uncoded refusals. A malformed or contradictory
	// element/publish receipt must retain its original submission uncertainty.
	if operation == "datasource.checkout" && receipt.Status == "unknown" && (receipt.Eligibility.Reason == "open_work_copy_limit" || receipt.Eligibility.Reason == "permission_denied") {
		receipt.Status = "rejected"
	}
}

func (c *Client) observedTDS(operation string, accepted bool, failure *value.TDSStatus, err error) value.TDSEligibility {
	e := c.TDSPrerequisites(operation)
	if e.RESTStatus != "supported" {
		return e
	}
	if accepted {
		e.Status, e.Reason = "supported", "operation_confirmed"
		if strings.HasPrefix(operation, "datasource.tds.read.") {
			e.Reason = "read_confirmed"
			return e
		}
		e.Permission, e.Entitlement = "supported", "supported"
		if e.SourceType != "" {
			e.SourceType, e.OpenCopyQuota = "supported", "supported"
		}
		return e
	}
	if failure != nil && failure.Code == http.StatusForbidden {
		// The documented HTTP 403 failed-element record is specific to this element.
		e.Permission, e.Status, e.Reason = "unsupported", "unsupported", "element_permission_denied"
		return e
	}
	up, ok := errors.AsType[*tableau.UpstreamError](err)
	if !ok {
		if tableau.SubmissionAttempted(err) {
			e.Reason = "response_inconclusive"
		}
		return e
	}
	e.Reason = "provider_refusal_unclassified"
	// Match only the documented Tableau condition or its explicit diagnostic.
	// A generic "Forbidden" can represent several different conditions.
	describes := func(messages ...string) bool {
		for _, message := range messages {
			if strings.EqualFold(strings.TrimSpace(up.Summary), message) || strings.EqualFold(strings.TrimSpace(up.Detail), message) {
				return true
			}
		}
		return false
	}
	switch {
	case operation == "datasource.checkout" && up.StatusCode == 405 && (up.Code == "405003" || up.Code == "405004"):
		e.SourceType, e.Status, e.Reason = "unsupported", "unsupported", "composable_datasource"
		if up.Code == "405004" {
			e.Reason = "semantic_data_model_datasource"
		}
	case up.StatusCode == 403 && up.Code == "403004" && describes("Unable to resolve tenant"):
		e.Reason = "entitlement_check_unavailable"
	case up.StatusCode == 403 && up.Code == "403004" && describes("Data Management Bundle not licensed", "No cloud entitlements", "Subscription not entitled"):
		e.Entitlement, e.Status, e.Reason = "unsupported", "unsupported", "missing_entitlement"
	case up.StatusCode == 403 && describes("Permission denied", "Insufficient permissions"):
		e.Permission, e.Status, e.Reason = "unsupported", "unsupported", "permission_denied"
	case up.StatusCode == 403:
		e.Reason = "forbidden_cause_unknown"
	case operation == "datasource.checkout" && up.StatusCode == 429 && up.Code == "" && describes("Work copy limit exceeded"):
		e.OpenCopyQuota, e.Status, e.Reason = "unsupported", "unsupported", "open_work_copy_limit"
	case up.StatusCode == 429 && up.Code == "429000":
		e.Reason = "rate_limited"
	case up.StatusCode == 404:
		e.Reason = "target_not_found_or_not_visible"
	case up.StatusCode >= 500:
		e.Reason = "provider_unavailable"
	}
	return e
}
