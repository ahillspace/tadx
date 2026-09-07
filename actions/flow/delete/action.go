package delete

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
}
type Deleter interface {
	DeleteFlow(context.Context, string) (Result, error)
}
type Action struct {
	resolver Resolver
	deleter  Deleter
}

func New(resolver Resolver, deleter Deleter) *Action {
	return &Action{resolver: resolver, deleter: deleter}
}
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, &errs.Error{ID: "flow.delete.unconfigured", Kind: errs.KindRuntime, Operation: "flow.delete", Summary: "Flow delete is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow delete before retrying."}
	}
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return Output{}, usage("environment", "flow delete requires an explicit resolved environment and site")
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.delete.resolve", Kind: errs.KindOperation, Operation: "flow.delete", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	plan := Plan{Mode: "preview", Operation: "flow.delete", Environment: input.Environment, Site: input.Site, Target: flow}
	output := Output{Plan: plan, Help: []string{"Run without --preview to delete this exact flow."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.delete.resolve", Kind: errs.KindOperation, Operation: "flow.delete", Environment: input.Environment, Site: input.Site, Summary: "Flow revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if current != flow {
		return Output{}, &errs.Error{ID: "flow.delete.target_changed", Kind: errs.KindOperation, Operation: "flow.delete", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "The flow delete target changed during revalidation.", Cause: errors.New("flow delete target changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before deleting."}
	}
	result, err := a.deleter.DeleteFlow(ctx, flow.LUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the upstream error before deleting again.")
		return Output{}, &errs.Error{ID: "flow.delete.failed", Kind: errs.KindOperation, Operation: "flow.delete", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow delete failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{"tadx content flow list"}
	return output, nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "flow.delete.usage", Kind: errs.KindUsage, Operation: "flow.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow delete input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
