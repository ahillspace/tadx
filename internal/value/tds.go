package value

import "regexp"

// TDS Edit's datasource and work-copy identifiers have a stricter contract than other Tableau LUIDs.
var tdsUUID = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$")

func ValidTDSLUID(s string) bool { return tdsUUID.MatchString(s) }

type TDSColumn struct {
	Caption *string `json:"caption,omitempty"`
	Role    *string `json:"role,omitempty"`
	Type    *string `json:"type,omitempty"`
}

// TDSStatus.Code is gRPC on reads and HTTP on edit failure receipts.
type TDSStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type TDSElement struct {
	ElementType string     `json:"elementType,omitempty"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Column      *TDSColumn `json:"column,omitempty"`
	Error       *TDSStatus `json:"error,omitempty"`
}

// TDSEligibility describes the evidence for this operation on this target and caller.
// An accepted operation does not identify a license SKU or establish future access.
type TDSEligibility struct {
	Operation     string `json:"operation"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	RESTVersion   string `json:"negotiated_rest_version,omitempty"`
	RESTStatus    string `json:"rest_version_status"`
	Permission    string `json:"permission"`
	Entitlement   string `json:"entitlement"`
	SourceType    string `json:"source_type,omitempty"`
	OpenCopyQuota string `json:"open_copy_quota,omitempty"`
}

type TDSRead struct {
	Eligibility      TDSEligibility `json:"eligibility,omitzero"`
	Elements         []TDSElement   `json:"elements"`
	Absent           bool           `json:"absent,omitempty"`
	TableauRequestID string         `json:"tableau_request_id,omitempty"`
}

// TDSReceipt retains provider-reported identities even when a response is contradictory.
type TDSReceipt struct {
	Eligibility      TDSEligibility `json:"eligibility,omitzero"`
	Status           string         `json:"status"`
	WorkCopyLUID     string         `json:"work_copy_luid,omitempty"`
	DatasourceLUID   string         `json:"datasource_luid,omitempty"`
	TableauRequestID string         `json:"tableau_request_id,omitempty"`
	Failure          *TDSStatus     `json:"failure,omitempty"`
}
