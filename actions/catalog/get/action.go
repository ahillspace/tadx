// Package get orchestrates exact reads from one local catalog generation.
package get

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

// Source resolves one exact local catalog record.
type Source interface {
	Get(context.Context, Input) (Result, error)
}

// Action orchestrates catalog.get.
type Action struct{ source Source }

// New creates catalog.get.
func New(source Source) *Action { return &Action{source: source} }

// Execute resolves one authoritative local identity.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.source == nil {
		return Output{}, actionError("catalog.get.unconfigured", errs.KindRuntime, input, "Catalog get is not configured.", nil)
	}
	input.LUID = strings.TrimSpace(input.LUID)
	input.Kind = strings.TrimSpace(input.Kind)
	input.Name = strings.TrimSpace(input.Name)
	input.ProjectPath = strings.TrimSpace(input.ProjectPath)
	if strings.TrimSpace(input.Environment) == "" || !input.SiteResolved || input.LUID == "" && (input.Kind == "" || input.Name == "") {
		return Output{}, actionError("catalog.get.usage", errs.KindUsage, input, "Catalog get requires a resolved site and a LUID or exact kind and name.", nil)
	}
	request := input
	result, err := a.source.Get(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, actionError("catalog.get.cancelled", errs.KindOperation, input, "Catalog get was canceled.", err)
		}
		var ambiguous interface{ AmbiguousCatalogSelector() bool }
		if errors.As(err, &ambiguous) && ambiguous.AmbiguousCatalogSelector() {
			return Output{}, actionError("catalog.get.ambiguous", errs.KindUsage, input, "Catalog selector matched more than one record.", err)
		}
		var notFound interface{ CatalogRecordNotFound() bool }
		if errors.As(err, &notFound) && notFound.CatalogRecordNotFound() {
			return Output{}, actionError("catalog.get.not_found", errs.KindUsage, input, "No catalog record matched the selector.", err)
		}
		var unavailable interface{ CatalogScopeUnavailable() bool }
		if errors.As(err, &unavailable) && unavailable.CatalogScopeUnavailable() {
			return Output{}, actionError("catalog.get.scope_unavailable", errs.KindUsage, input, "The requested content kind is not in the current catalog generation; refresh that scope.", err)
		}
		return Output{}, actionError("catalog.get.failed", errs.KindOperation, input, "Catalog get failed.", err)
	}
	return Output{Item: result.Item, Generation: result.Generation, Warnings: output.BoundWarnings(result.Warnings), Help: []string{"tadx content get --kind " + result.Item.Kind + " --id " + result.Item.LUID}}, nil
}

func actionError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "catalog.get", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Choose one exact catalog identity, then retry."}
}
