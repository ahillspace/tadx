// Package status reports bounded local catalog generation status.
package status

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

// Source reads one local catalog generation status.
type Source interface {
	Status(context.Context, Input) (Result, error)
}

// Action orchestrates catalog.status.
type Action struct{ source Source }

// New creates catalog.status.
func New(source Source) *Action { return &Action{source: source} }

// Execute returns current generation age, completeness, source, and stale state.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.source == nil {
		return Output{}, statusError("catalog.status.unconfigured", errs.KindRuntime, input, "Catalog status is not configured.", nil)
	}
	if strings.TrimSpace(input.Environment) == "" || !input.SiteResolved {
		return Output{}, statusError("catalog.status.usage", errs.KindUsage, input, "Catalog status requires a resolved environment and site.", nil)
	}
	request := input
	result, err := a.source.Status(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, statusError("catalog.status.cancelled", errs.KindOperation, input, "Catalog status was canceled.", err)
		}
		return Output{}, statusError("catalog.status.failed", errs.KindOperation, input, "Catalog status failed.", err)
	}
	state := "current"
	if result.Stale {
		state = "stale"
	}
	if !result.Complete {
		state = "partial"
	}
	if result.ID == "" {
		state = "uninitialized"
	}
	generation := Generation{ID: result.ID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt, Records: result.Records, Complete: result.Complete, Stale: result.Stale, Age: result.Age, Source: result.Source}
	return Output{Status: state, Generation: generation, Path: result.Path, Warnings: output.BoundWarnings(result.Warnings), Help: []string{commandhint.Environment(result.Environment, "catalog", "refresh")}}, nil
}

func statusError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "catalog.status", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Refresh or repair the selected catalog generation, then retry."}
}
