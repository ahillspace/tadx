// Package cancel owns supported exact-job cancellation and confirmation.
package cancel

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// Source performs preview, cancellation, and bounded exact confirmation.
type Source interface {
	Cancel(context.Context, Input) (Result, error)
}

// Action validates and projects job cancellation results.
type Action struct{ source Source }

// New creates a job.cancel action.
func New(source Source) *Action { return &Action{source: source} }

// Execute requests cancellation only for the supported job types selected by
// the source. Preview never authorizes or performs a remote cancellation.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.source == nil {
		return Output{}, cancelError("job.cancel.unconfigured", errs.KindRuntime, input, "Job cancellation is not configured.", nil, errs.OutcomeNotAttempted)
	}
	result, err := a.source.Cancel(ctx, input)
	if err != nil {
		if _, ok := errors.AsType[*errs.Error](err); ok {
			return project(result, input), err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return project(result, input), cancelError("job.cancel.cancelled", errs.KindOperation, input, "Job cancellation did not establish the remote outcome.", err, errs.OutcomeUnknown)
		}
		return project(result, input), cancelError("job.cancel.failed", errs.KindOperation, input, "Job cancellation failed without a confirmed remote outcome.", err, errs.OutcomeUnknown)
	}
	return project(result, input), nil
}

func project(result Result, input Input) Output {
	help := []string{commandhint.Environment(result.Environment, "job", "inspect", "--id", input.ID)}
	status := result.Status.Status
	if input.Preview {
		status = "preview"
	}
	if status == "" {
		status = "cancellation_requested"
	}
	return Output{Status: status, Environment: result.Environment, Site: result.Site, Job: result.Status, RequestID: result.RequestID, Confirmed: result.Confirmed, Warnings: append([]string(nil), result.Warnings...), Help: help}
}

func cancelError(id string, kind errs.Kind, input Input, summary string, cause error, outcome errs.Outcome) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "job.cancel", Resource: input.ID, Environment: input.Environment, Site: input.Site, TableauJobID: input.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact job identity before attempting any further cancellation; do not infer a terminal state from an acknowledgement alone.", Phase: errs.PhaseVerification, Outcome: outcome}
}
