// Package inspect owns exact Tableau job inspection orchestration.
package inspect

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// Source performs one exact job inspection.
type Source interface {
	Inspect(context.Context, Input) (Result, error)
}

// Action validates and projects job inspection results.
type Action struct{ source Source }

// New creates a job.inspect action.
func New(source Source) *Action { return &Action{source: source} }

// Execute inspects the requested exact job without changing remote state.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.source == nil {
		return Output{}, jobError("job.inspect.unconfigured", errs.KindRuntime, input, "Job inspection is not configured.", nil)
	}
	result, err := a.source.Inspect(ctx, input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, jobError("job.inspect.cancelled", errs.KindOperation, input, "Job inspection was canceled before an authoritative result was returned.", err)
		}
		var structured *errs.Error
		if errors.As(err, &structured) {
			return Output{}, err
		}
		return Output{}, jobError("job.inspect.failed", errs.KindOperation, input, "Job inspection failed.", err)
	}
	if result.Status.ID != input.ID || result.Status.Status == "" {
		return Output{}, jobError("job.inspect.identity", errs.KindOperation, input, "Job inspection did not return the requested exact identity and state.", nil)
	}
	environment, site := result.Environment, result.Site
	if environment == "" {
		environment = input.Environment
	}
	if site == "" {
		site = input.Site
	}
	return Output{
		Status:      result.Status.Status,
		Environment: environment,
		Site:        site,
		Job:         result.Status,
		Attempts:    result.Attempts,
		Warnings:    append([]string(nil), result.Warnings...),
		Help:        []string{},
	}, nil
}

func jobError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "job.inspect", Resource: input.ID, Environment: input.Environment, Site: input.Site, TableauJobID: input.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Retry the exact job inspection or recover the saved job receipt.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
}
