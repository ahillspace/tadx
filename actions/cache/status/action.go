// Package status reports bounded local cache generation status.
package status

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

// Source reads one local cache generation status.
type Source interface {
	Status(context.Context, Input) (Result, error)
}

// Action orchestrates cache.status.
type Action struct{ source Source }

// New creates cache.status.
func New(source Source) *Action { return &Action{source: source} }

// Execute returns current generation age, completeness, source, and stale state.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.source == nil {
		return Output{}, statusError("cache.status.unconfigured", errs.KindRuntime, input, "Cache status is not configured.", nil)
	}
	if strings.TrimSpace(input.Environment) == "" || !input.SiteResolved {
		return Output{}, statusError("cache.status.usage", errs.KindUsage, input, "Cache status requires a resolved environment and site.", nil)
	}
	request := input
	result, err := a.source.Status(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, statusError("cache.status.cancelled", errs.KindOperation, input, "Cache status was canceled.", err)
		}
		return Output{}, statusError("cache.status.failed", errs.KindOperation, input, "Cache status failed.", err)
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
	generation.Coverage = result.Coverage
	return Output{Retained: result.Retained, Status: state, Generation: generation, Path: result.Path, Warnings: output.BoundWarnings(result.Warnings), Help: []string{commandhint.Environment(result.Environment, "cache", "refresh")}}, nil
}

func statusError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	advice := "Refresh or repair the selected cache generation, then retry."
	var incompatible interface{ CacheSchemaIncompatible() bool }
	if errors.As(cause, &incompatible) && incompatible.CacheSchemaIncompatible() {
		id = "cache.status.schema_incompatible"
		advice = "Preserve the cache and repair its schema or use a compatible TADX build; identical refresh attempts cannot repair inconsistent schema markers."
	}
	return &errs.Error{ID: id, Kind: kind, Operation: "cache.status", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: advice}
}
