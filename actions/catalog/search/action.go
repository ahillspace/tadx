package search

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

const maxLimit = 100

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
	if input.Limit < 0 || input.Limit > maxLimit {
		message := "Catalog search limit must be nonnegative and at most 100."
		return Output{}, &errs.Error{
			ID: "catalog.search.usage", Kind: errs.KindUsage, Operation: "catalog.search", Summary: message,
			Validation: []errs.ValidationDetail{{Field: "limit", Code: "range", Message: message}},
		}
	}
	result, err := a.source.Search(ctx, input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, &errs.Error{ID: "catalog.search.cancelled", Kind: errs.KindOperation, Operation: "catalog.search", Environment: input.Environment, Site: input.Site, Summary: "Catalog search was canceled.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Run the catalog search again when ready."}
		}
		var invalidCursor interface{ InvalidCatalogCursor() bool }
		if errors.As(err, &invalidCursor) && invalidCursor.InvalidCatalogCursor() {
			message := "Catalog search cursor is invalid."
			return Output{}, &errs.Error{
				ID: "catalog.search.usage", Kind: errs.KindUsage, Operation: "catalog.search", Summary: message, Cause: err,
				Validation: []errs.ValidationDetail{{Field: "cursor", Code: "invalid", Message: message}},
			}
		}
		return Output{}, &errs.Error{ID: "catalog.search.failed", Kind: errs.KindOperation, Operation: "catalog.search", Environment: input.Environment, Site: input.Site, Summary: "Catalog search failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Refresh or repair the selected catalog generation, then retry."}
	}
	return Output{
		Page:       result.Page,
		Generation: Generation{ID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt, Stale: result.Stale},
		Items:      append([]Item{}, result.Items...), Warnings: append([]string(nil), result.Warnings...), Help: []string{"tadx catalog search --environment <alias> --id <luid>"},
	}, nil
}
