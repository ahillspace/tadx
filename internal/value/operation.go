package value

import "encoding/json"

// OperationItem is one bounded local publication outcome. Result is the
// decoded saved item projection when the worker supplied one.
type OperationItem struct {
	Key              string `json:"key,omitempty"`
	Status           string `json:"status,omitempty"`
	JobID            string `json:"tableau_job_id,omitempty"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
	ResourceID       string `json:"resource_id,omitempty"`
	Name             string `json:"name,omitempty"`
	Project          string `json:"project,omitempty"`
	ReceiptPath      string `json:"receipt_path,omitempty"`
	Error            string `json:"error,omitempty"`
	Verification     string `json:"verification,omitempty"`
	Result           any    `json:"result,omitempty"`
}

// OperationRecord is the non-secret snapshot input, independent of durable storage.
type OperationRecord struct {
	ID, Operation, Phase, ConfigPath       string
	CompactResult, FullResult, LiveResults json.RawMessage
}
