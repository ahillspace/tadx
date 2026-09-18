// Package wait owns durable exact-job monitoring without resubmission.
package wait

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// Source recovers or begins observing one exact job without submitting work.
type Source interface {
	Wait(context.Context, Input) (Result, error)
}

// Action validates and projects durable job recovery.
type Action struct{ source Source }

// New creates a job.wait action.
func New(source Source) *Action { return &Action{source: source} }

// Execute waits for authoritative terminal state or returns an honest
// interruption/unknown outcome from local job tracking.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.source == nil {
		return Output{}, waitError("job.wait.unconfigured", errs.KindRuntime, input, "Job recovery is not configured.", nil)
	}
	result, err := a.source.Wait(ctx, input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return project(result, input), waitError("job.wait.cancelled", errs.KindOperation, input, "Job monitoring stopped before an authoritative terminal result was returned.", err)
		}
		var structured *errs.Error
		if errors.As(err, &structured) {
			return project(result, input), err
		}
		return project(result, input), waitError("job.wait.failed", errs.KindOperation, input, "Job recovery failed without changing the accepted remote job.", err)
	}
	if result.Status.ID == "" || result.Status.Status == "" {
		return Output{}, waitError("job.wait.identity", errs.KindOperation, input, "Job recovery did not return an exact authoritative state.", nil)
	}
	return project(result, input), nil
}

func project(result Result, input Input) Output {
	help := []string{commandhint.Environment(result.Environment, "job", "inspect", "--id", result.Status.ID)}
	if !result.Status.Terminal() {
		help = append(help, commandhint.Environment(result.Environment, "job", "wait", "--id", result.Status.ID))
	}
	return Output{Status: result.Status.Status, Environment: result.Environment, Site: result.Site, Job: result.Status, ReceiptPath: result.ReceiptPath, Warnings: append([]string(nil), result.Warnings...), Help: help}
}

func waitError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "job.wait", Resource: input.ID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Use the saved receipt or exact job identity to inspect the authoritative outcome; do not resubmit the original request.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
}
