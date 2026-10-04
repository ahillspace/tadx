package job

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// WaitInput recovers one accepted job or begins observing one exact job ID.
type WaitInput struct {
	Environment string
	Site        string
	ID          string
	Receipt     string
}

// WaitResult is the source-facing durable monitoring result.
type WaitResult struct {
	Status      value.JobStatus
	Environment string
	Site        string
	ReceiptPath string
	Warnings    []string
}

// WaitOutput is the stable job.wait document.
type WaitOutput struct {
	Status      string          `json:"status"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Job         value.JobStatus `json:"job"`
	ReceiptPath string          `json:"receipt_path,omitempty"`
	Warnings    []string        `json:"warnings,omitempty"`
	Help        []string        `json:"help"`
}

// ValidateWaitInput requires one durable receipt or one exact job ID.
func ValidateWaitInput(input WaitInput) error {
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

// Wait waits for authoritative terminal state or returns an honest
// interruption/unknown outcome from local job tracking.
func waitOutput(ctx context.Context, wait func(context.Context, WaitInput) (WaitResult, error), input WaitInput) (WaitOutput, error) {
	if err := ValidateWaitInput(input); err != nil {
		return WaitOutput{}, err
	}
	if wait == nil {
		return WaitOutput{}, waitError("job.wait.unconfigured", errs.KindRuntime, input, "Job recovery is not configured.", nil)
	}
	result, err := wait(ctx, input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return projectWait(result, input), waitError("job.wait.cancelled", errs.KindOperation, input, "Job monitoring stopped before an authoritative terminal result was returned.", err)
		}
		if _, ok := errors.AsType[*errs.Error](err); ok {
			return projectWait(result, input), err
		}
		return projectWait(result, input), waitError("job.wait.failed", errs.KindOperation, input, "Job recovery failed without changing the accepted remote job.", err)
	}
	if result.Status.ID == "" || result.Status.Status == "" {
		return WaitOutput{}, waitError("job.wait.identity", errs.KindOperation, input, "Job recovery did not return an exact authoritative state.", nil)
	}
	return projectWait(result, input), nil
}

func projectWait(result WaitResult, input WaitInput) WaitOutput {
	help := []string{commandhint.Environment(result.Environment, "job", "inspect", "--id", result.Status.ID)}
	if !result.Status.Terminal() {
		help = append(help, commandhint.Environment(result.Environment, "job", "wait", "--id", result.Status.ID))
	}
	return WaitOutput{Status: result.Status.Status, Environment: result.Environment, Site: result.Site, Job: result.Status, ReceiptPath: result.ReceiptPath, Warnings: append([]string(nil), result.Warnings...), Help: help}
}

func waitError(id string, kind errs.Kind, input WaitInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "job.wait", Resource: input.ID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Use the saved receipt or exact job identity to inspect the authoritative outcome; do not resubmit the original request.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
}
