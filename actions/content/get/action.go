// Package get defines the gated action seam for resource-specific exact content reads.
package get

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

// Resolver delegates exact reads to the selected resource adapter.
type Resolver interface {
	Get(context.Context, Input) (Result, error)
}

// Action orchestrates content.get without implementing upstream protocols.
type Action struct{ resolver Resolver }

// New creates content.get.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative remote identity.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, getError("content.get.unconfigured", errs.KindRuntime, input, "Content get is not configured.", nil)
	}
	input.Kind = strings.TrimSpace(input.Kind)
	input.LUID = strings.TrimSpace(input.LUID)
	input.Name = strings.TrimSpace(input.Name)
	input.ProjectPath = strings.TrimSpace(input.ProjectPath)
	if input.Kind == "" || input.LUID == "" && input.Name == "" {
		return Output{}, getError("content.get.usage", errs.KindUsage, input, "Content get requires a resource kind and a LUID or exact name.", nil)
	}
	if input.LUID != "" && (input.Name != "" || input.ProjectPath != "") {
		return Output{}, getError("content.get.usage", errs.KindUsage, input, "A LUID is authoritative and cannot be combined with name or project selectors.", nil)
	}
	if input.Environment != "" && !input.SiteResolved {
		return Output{}, getError("content.get.usage", errs.KindUsage, input, "Content get requires a resolved source site.", nil)
	}
	request := input
	result, err := a.resolver.Get(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, getError("content.get.cancelled", errs.KindOperation, input, "Content get was canceled.", err)
		}
		var ambiguous interface{ AmbiguousContentSelector() bool }
		if errors.As(err, &ambiguous) && ambiguous.AmbiguousContentSelector() {
			return Output{}, getError("content.get.ambiguous", errs.KindUsage, input, "Content selector matched more than one item.", err)
		}
		return Output{}, getError("content.get.failed", errs.KindOperation, input, "Content get failed.", err)
	}
	if result.Item.LUID == "" || result.Item.Kind == "" || result.Item.Name == "" || result.Item.Kind != input.Kind || input.LUID != "" && result.Item.LUID != input.LUID {
		return Output{}, getError("content.get.failed", errs.KindOperation, input, "Content adapter returned a mismatched authoritative identity.", nil)
	}
	return Output{Item: result.Item, Warnings: output.BoundWarnings(result.Warnings), Help: []string{"tadx capability get " + result.Item.Kind + ".get --full"}}, nil
}

func getError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "content.get", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Choose one exact supported content identity, then retry."}
}
