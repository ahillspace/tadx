package move

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
	ResolveProject(context.Context, identity.Selector) (Project, error)
}
type Mover interface {
	MoveFlow(context.Context, string, string) (Result, error)
}
type Action struct {
	resolver Resolver
	mover    Mover
}

func New(resolver Resolver, mover Mover) *Action { return &Action{resolver: resolver, mover: mover} }
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	ctx = a.beginProjectResolution(ctx)
	if a == nil || a.resolver == nil || a.mover == nil {
		return Output{}, &errs.Error{ID: "flow.move.unconfigured", Kind: errs.KindRuntime, Operation: "flow.move", Summary: "Flow move is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow move before retrying."}
	}
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return Output{}, usage("environment", "flow move requires an explicit resolved environment and site")
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.FlowSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.move.resolve", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project, then retry.")
		return Output{}, &errs.Error{ID: "flow.move.project", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Destination project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	plan := Plan{Mode: "preview", Operation: "flow.move", Environment: input.Environment, Site: input.Site, Source: flow, Destination: project, NoOp: flow.ProjectLUID == project.LUID}
	output := Output{Plan: plan, Help: []string{"Run without --preview to move this exact flow."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	ctx = a.beginProjectResolution(ctx)
	current, err := a.resolver.ResolveFlow(ctx, input.FlowSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.move.resolve", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Flow revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	destination, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project, then retry.")
		return Output{}, &errs.Error{ID: "flow.move.project", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Destination project revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if current != flow || destination != project {
		return Output{}, &errs.Error{ID: "flow.move.target_changed", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "The flow move source or destination changed during revalidation.", Cause: errors.New("flow move source or destination changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before moving."}
	}
	if plan.NoOp {
		output.Result = &Result{Status: "unchanged", FlowLUID: flow.LUID, ProjectLUID: project.LUID}
		return output, nil
	}
	result, err := a.mover.MoveFlow(ctx, flow.LUID, project.LUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the upstream error before moving again.")
		return Output{}, &errs.Error{ID: "flow.move.failed", Kind: errs.KindOperation, Operation: "flow.move", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow move failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{"tadx content flow inspect --id " + flow.LUID}
	return output, nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "flow.move.usage", Kind: errs.KindUsage, Operation: "flow.move", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
