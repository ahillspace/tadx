package wait

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// Input recovers one accepted job or begins observing one exact job ID.
type Input struct {
	Environment string
	Site        string
	ID          string
	Receipt     string
}

// Result is the source-facing durable monitoring result.
type Result struct {
	Status      value.JobStatus
	Environment string
	Site        string
	ReceiptPath string
	Warnings    []string
}

// Output is the stable job.wait document.
type Output struct {
	Status      string          `json:"status"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Job         value.JobStatus `json:"job"`
	ReceiptPath string          `json:"receipt_path,omitempty"`
	Warnings    []string        `json:"warnings,omitempty"`
	Help        []string        `json:"help"`
}

// ValidateInput requires one durable receipt or one exact job ID.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.ID) == "" && strings.TrimSpace(input.Receipt) == "" {
		return &errs.Error{ID: "job.wait.usage", Kind: errs.KindUsage, Operation: "job.wait", Summary: "Job wait requires an exact job ID or durable receipt path.", Retryable: errs.Bool(false), CorrectiveAction: "Provide --id <job-id> or --receipt <receipt-path>."}
	}
	if strings.TrimSpace(input.ID) != "" && input.ID != strings.TrimSpace(input.ID) {
		return &errs.Error{ID: "job.wait.usage", Kind: errs.KindUsage, Operation: "job.wait", Summary: "The exact Tableau job ID must not contain surrounding whitespace.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact job ID with --id."}
	}
	if strings.TrimSpace(input.Receipt) != "" && input.Receipt != strings.TrimSpace(input.Receipt) {
		return &errs.Error{ID: "job.wait.usage", Kind: errs.KindUsage, Operation: "job.wait", Summary: "The durable receipt path must not contain surrounding whitespace.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a path returned by an accepted publication."}
	}
	if strings.TrimSpace(input.ID) != "" && strings.TrimSpace(input.Receipt) != "" {
		return &errs.Error{ID: "job.wait.selector", Kind: errs.KindUsage, Operation: "job.wait", Summary: "Choose an exact job ID or a durable receipt path, not both.", Retryable: errs.Bool(false), CorrectiveAction: "Provide only --id or --receipt."}
	}
	return nil
}
