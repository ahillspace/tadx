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
	OperationID string
	Preview     bool
}

// CancelResult records the request acknowledgement and bounded exact confirmation.
type CancelResult struct {
	OperationID   string
	ControlStatus string
	Outcome       any
	Status        value.JobStatus
	Environment   string
	Site          string
	RequestID     string
	Confirmed     bool
	Warnings      []string
}

// CancelOutput is the stable job.cancel document.
type CancelOutput struct {
	OperationID string          `json:"operation_id,omitempty"`
	Outcome     any             `json:"outcome,omitempty"`
	Status      string          `json:"status"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Job         value.JobStatus `json:"job,omitzero"`
	RequestID   string          `json:"request_id,omitempty"`
	Confirmed   bool            `json:"confirmed"`
	Warnings    []string        `json:"warnings,omitempty"`
	Help        []string        `json:"help"`
}

// ValidateCancelInput checks exact identity before any authentication or remote call.
func ValidateCancelInput(input CancelInput) error {
	id, operationID := strings.TrimSpace(input.ID), strings.TrimSpace(input.OperationID)
	if (id == "") == (operationID == "") || input.ID != id || input.OperationID != operationID || strings.ContainsAny(id+operationID, "\r\n") {
		return &errs.Error{ID: "job.cancel.usage", Kind: errs.KindUsage, Operation: "job.cancel", Summary: "Choose one exact Tableau job ID or operation ID.", Retryable: errs.Bool(false), CorrectiveAction: "Provide exactly one of --id or --operation-id."}
	}
	if operationID != "" && (input.Preview || input.Environment != "" || input.Site != "") {
		return &errs.Error{ID: "job.cancel.usage", Kind: errs.KindUsage, Operation: "job.cancel", Summary: "Local operation cancellation does not accept target or preview flags.", Retryable: errs.Bool(false)}
	}
	return nil
}

// Cancel requests cancellation only for the supported job types selected by
// the source. Preview never authorizes or performs a remote cancellation.
func Cancel(ctx context.Context, cancel func(context.Context, CancelInput) (CancelResult, error), input CancelInput) (CancelOutput, error) {
	if err := ValidateCancelInput(input); err != nil {
		return CancelOutput{}, err
	}
	if cancel == nil {
		return CancelOutput{}, cancelError("job.cancel.unconfigured", errs.KindRuntime, input, "Job cancellation is not configured.", nil, errs.OutcomeNotAttempted)
	}
	result, err := cancel(ctx, input)
	if err != nil {
		if input.OperationID != "" {
			return projectCancel(result, input), &errs.Error{ID: "job.cancel.local", Kind: errs.KindOperation, Operation: "job.cancel", Resource: input.OperationID, Summary: "Local cancellation could not be confirmed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact operation and its saved outcome."}
		}
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
	if input.OperationID != "" {
		return CancelOutput{OperationID: input.OperationID, Outcome: result.Outcome, Status: result.ControlStatus, Confirmed: result.Confirmed, Warnings: result.Warnings, Help: []string{commandhint.Command("job", "inspect", "--operation-id", input.OperationID)}}
	}
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
