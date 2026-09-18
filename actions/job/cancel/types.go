package cancel

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// Input requests cancellation of one exact supported Tableau job.
type Input struct {
	Environment string
	Site        string
	ID          string
	Preview     bool
}

// Result records the request acknowledgement and bounded exact confirmation.
type Result struct {
	Status      value.JobStatus
	Environment string
	Site        string
	RequestID   string
	Confirmed   bool
	Warnings    []string
}

// Output is the stable job.cancel document.
type Output struct {
	Status      string          `json:"status"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Job         value.JobStatus `json:"job"`
	RequestID   string          `json:"request_id,omitempty"`
	Confirmed   bool            `json:"confirmed"`
	Warnings    []string        `json:"warnings,omitempty"`
	Help        []string        `json:"help"`
}

// ValidateInput checks exact identity before any authentication or remote call.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.ID) == "" || input.ID != strings.TrimSpace(input.ID) {
		return &errs.Error{ID: "job.cancel.usage", Kind: errs.KindUsage, Operation: "job.cancel", Summary: "An exact Tableau job ID is required.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact job ID with --id."}
	}
	if strings.ContainsAny(input.ID, "\r\n") {
		return &errs.Error{ID: "job.cancel.usage", Kind: errs.KindUsage, Operation: "job.cancel", Summary: "The Tableau job ID must be a single line.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact job ID with --id."}
	}
	return nil
}
