package inspect

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// Input selects one exact Tableau job for inspection.
type Input struct {
	Environment string
	Site        string
	ID          string
}

// Result is the source-facing inspection result.
type Result struct {
	Status      value.JobStatus
	Environment string
	Site        string
	Attempts    int
	Warnings    []string
}

// Output is the stable job.inspect document.
type Output struct {
	Status      string          `json:"status"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Job         value.JobStatus `json:"job"`
	Attempts    int             `json:"attempts,omitempty"`
	Warnings    []string        `json:"warnings,omitempty"`
	Help        []string        `json:"help"`
}

// ValidateInput checks exact identity.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.ID) == "" || input.ID != strings.TrimSpace(input.ID) {
		return &errs.Error{ID: "job.inspect.usage", Kind: errs.KindUsage, Operation: "job.inspect", Summary: "An exact Tableau job ID is required.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact job ID with --id."}
	}
	if strings.ContainsAny(input.ID, "\r\n") {
		return &errs.Error{ID: "job.inspect.usage", Kind: errs.KindUsage, Operation: "job.inspect", Summary: "The Tableau job ID must be a single line.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact job ID with --id."}
	}
	return nil
}
