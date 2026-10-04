package job

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// CancelInput requests cancellation of one exact supported Tableau job.
type CancelInput struct {
	Environment string
	Site        string
	ID          string
	Preview     bool
}

// CancelResult records the request acknowledgement and bounded exact confirmation.
type CancelResult struct {
	Status      value.JobStatus
	Environment string
	Site        string
	RequestID   string
	Confirmed   bool
	Warnings    []string
}

// CancelOutput is the stable job.cancel document.
type CancelOutput struct {
	Status      string          `json:"status"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Job         value.JobStatus `json:"job"`
	RequestID   string          `json:"request_id,omitempty"`
	Confirmed   bool            `json:"confirmed"`
	Warnings    []string        `json:"warnings,omitempty"`
	Help        []string        `json:"help"`
}

// ValidateCancelInput checks exact identity before any authentication or remote call.
func ValidateCancelInput(input CancelInput) error {
	if strings.TrimSpace(input.ID) == "" || input.ID != strings.TrimSpace(input.ID) {
		return &errs.Error{ID: "job.cancel.usage", Kind: errs.KindUsage, Operation: "job.cancel", Summary: "An exact Tableau job ID is required.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact job ID with --id."}
	}
	if strings.ContainsAny(input.ID, "\r\n") {
		return &errs.Error{ID: "job.cancel.usage", Kind: errs.KindUsage, Operation: "job.cancel", Summary: "The Tableau job ID must be a single line.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact job ID with --id."}
	}
	return nil
}

// cancelOutput requests cancellation only for the supported job types selected by
// the source. Preview never authorizes or performs a remote cancellation.
func cancelOutput(ctx context.Context, cancel func(context.Context, CancelInput) (CancelResult, error), input CancelInput) (CancelOutput, error) {
	if err := ValidateCancelInput(input); err != nil {
		return CancelOutput{}, err
	}
	if cancel == nil {
		return CancelOutput{}, cancelError("job.cancel.unconfigured", errs.KindRuntime, input, "Job cancellation is not configured.", nil, errs.OutcomeNotAttempted)
	}
	result, err := cancel(ctx, input)
	if err != nil {
		if _, ok := errors.AsType[*errs.Error](err); ok {
			return projectCancel(result, input), err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return projectCancel(result, input), cancelError("job.cancel.cancelled", errs.KindOperation, input, "Job cancellation did not establish the remote outcome.", err, errs.OutcomeUnknown)
		}
		return projectCancel(result, input), cancelError("job.cancel.failed", errs.KindOperation, input, "Job cancellation failed without a confirmed remote outcome.", err, errs.OutcomeUnknown)
	}
	return projectCancel(result, input), nil
}

func projectCancel(result CancelResult, input CancelInput) CancelOutput {
	help := []string{commandhint.Environment(result.Environment, "job", "inspect", "--id", input.ID)}
	status := result.Status.Status
	if input.Preview {
		status = "preview"
	}
	if status == "" {
		status = "cancellation_requested"
	}
	return CancelOutput{Status: status, Environment: result.Environment, Site: result.Site, Job: result.Status, RequestID: result.RequestID, Confirmed: result.Confirmed, Warnings: append([]string(nil), result.Warnings...), Help: help}
}

func cancelError(id string, kind errs.Kind, input CancelInput, summary string, cause error, outcome errs.Outcome) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "job.cancel", Resource: input.ID, Environment: input.Environment, Site: input.Site, TableauJobID: input.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact job identity before attempting any further cancellation; do not infer a terminal state from an acknowledgement alone.", Phase: errs.PhaseVerification, Outcome: outcome}
}
