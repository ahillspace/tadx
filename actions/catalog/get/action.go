// Package get orchestrates exact reads from one local catalog generation.
package get

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	maxWarnings     = 20
	maxWarningRunes = 512
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
		return Output{}, actionError("catalog.get.failed", errs.KindOperation, input, "Catalog get failed.", err)
	}
	return Output{Item: result.Item, Generation: result.Generation, Warnings: bounded(result.Warnings), Help: []string{"tadx content get --kind " + result.Item.Kind + " --id " + result.Item.LUID}}, nil
}

func actionError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "catalog.get", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Choose one exact catalog identity, then retry."}
}

func bounded(values []string) []string {
	if len(values) > maxWarnings {
		values = values[:maxWarnings]
	}
	bounded := make([]string, len(values))
	for index, value := range values {
		runes := []rune(value)
		if len(runes) > maxWarningRunes {
			value = string(runes[:maxWarningRunes-3]) + "..."
		}
		bounded[index] = value
	}
	return bounded
}
