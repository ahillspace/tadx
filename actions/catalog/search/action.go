package search

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
)

// Source searches one complete normalized local catalog generation.
type Source interface {
	Search(context.Context, Input) (Result, error)
}

// Action orchestrates catalog.search.
type Action struct{ source Source }

// New creates catalog.search.
func New(source Source) *Action { return &Action{source: source} }

// Execute returns one bounded stable local page.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.source == nil {
		return Output{}, errs.New(errs.KindRuntime, "Catalog search is not configured.")
	}
	result, err := a.source.Search(ctx, input)
	if err != nil {
		return Output{}, &errs.Error{ID: "catalog.search.failed", Kind: errs.KindOperation, Operation: "catalog.search", Environment: input.Environment, Site: input.Site, Summary: "Catalog search failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Refresh or repair the selected catalog generation, then retry."}
	}
	help := fmt.Sprintf("tadx catalog get --environment %s --id <luid>", input.Environment)
	return Output{
		Page:       result.Page,
		Generation: Generation{ID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt, Stale: result.Stale},
		Items:      append([]Item(nil), result.Items...), Warnings: append([]string(nil), result.Warnings...), Help: []string{help},
	}, nil
}
