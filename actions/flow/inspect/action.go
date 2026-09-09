package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned flow seam.
type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
}
type Action struct{ resolver Resolver }

func New(resolver Resolver) *Action { return &Action{resolver: resolver} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "flow.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "flow.inspect", Summary: "Flow inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the flow resolver before retrying."}
	}
	var validationErr error
	input, validationErr = normalizeInput(input)
	if validationErr != nil {
		return Output{}, validationErr
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.inspect.resolve", Kind: errs.KindOperation, Operation: "flow.inspect", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Flow: flow, RequestID: flow.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "flow", "pull", "--id", flow.LUID)}}, nil
}
