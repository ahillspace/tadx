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

type TDSRead struct {
	Elements         []TDSElement `json:"elements"`
	Absent           bool         `json:"absent,omitempty"`
	TableauRequestID string       `json:"tableau_request_id,omitempty"`
}

// TDSReceipt retains provider-reported identities even when a response is contradictory.
type TDSReceipt struct {
	Status           string     `json:"status"`
	WorkCopyLUID     string     `json:"work_copy_luid,omitempty"`
	DatasourceLUID   string     `json:"datasource_luid,omitempty"`
	TableauRequestID string     `json:"tableau_request_id,omitempty"`
	Failure          *TDSStatus `json:"failure,omitempty"`
}
