package get

import (
	"context"
	"errors"

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
		return Output{}, &errs.Error{ID: "flow.get.unconfigured", Kind: errs.KindRuntime, Operation: "flow.get", Summary: "Flow get is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the flow resolver before retrying."}
	}
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return Output{}, &errs.Error{ID: "flow.get.usage", Kind: errs.KindUsage, Operation: "flow.get", Summary: "flow selection requires a LUID or exact name and project path", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "flow selection requires a LUID or exact name and project path"}}}
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.get.resolve", Kind: errs.KindOperation, Operation: "flow.get", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Flow: flow, RequestID: flow.RequestID, Help: []string{"tadx content flow pull --id " + flow.LUID}}, nil
}
