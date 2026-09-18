package value

import "time"

// JobStatus is an authoritative observation, independent of local monitoring.
type JobStatus struct {
	ID         string    `json:"id"`
	Type       string    `json:"type,omitempty"`
	Status     string    `json:"status"`
	Progress   *int      `json:"progress,omitempty"`
	FinishCode *int      `json:"finish_code,omitempty"`
	ResourceID string    `json:"resource_id,omitempty"`
	CheckedAt  time.Time `json:"checked_at,omitzero"`
	RequestID  string    `json:"request_id,omitempty"`
}

func (s JobStatus) Terminal() bool {
	return s.Status == "succeeded" || s.Status == "failed" || s.Status == "cancelled"
}
