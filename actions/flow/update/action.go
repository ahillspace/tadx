package update

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
}
type Updater interface {
	UpdateFlow(context.Context, Request) (Result, error)
}
type Action struct {
	resolver Resolver
	updater  Updater
}

func New(r Resolver, u Updater) *Action { return &Action{r, u} }
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.updater == nil {
		return Output{}, &errs.Error{ID: "flow.update.unconfigured", Kind: errs.KindRuntime, Operation: "flow.update", Summary: "Flow update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow update before retrying."}
	}
	if err := validate(in); err != nil {
		return Output{}, err
	}
	target, err := a.resolver.ResolveFlow(ctx, in.Selector)
	if err != nil {
		return Output{}, operationError("flow.update.resolve", in, "", "Flow resolution failed.", "Review the exact flow selector, then retry.", err)
	}
	request, changes := changedRequest(target, in)
	out := Output{Plan: Plan{Mode: "preview", Operation: "flow.update", Environment: in.Environment, Site: in.Site, Target: target, Changes: changes, NoOp: len(changes) == 0}, Help: []string{"Run without --preview to update this exact flow owner."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.resolver.ResolveFlow(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return Output{}, operationError("flow.update.resolve", in, target.LUID, "Flow revalidation failed.", "Review a new preview before updating.", err)
	}
	if current.Name != target.Name || current.ProjectLUID != target.ProjectLUID || current.OwnerLUID != target.OwnerLUID {
		return Output{}, operationError("flow.update.target_changed", in, target.LUID, "The flow changed during revalidation.", "Review a new preview before updating.", errors.New("flow identity changed during revalidation"))
	}
	request, changes = changedRequest(current, in)
	out.Plan.Target, out.Plan.Changes, out.Plan.NoOp = current, changes, len(changes) == 0
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", FlowLUID: current.LUID, FlowName: current.Name, ProjectLUID: current.ProjectLUID, OwnerLUID: current.OwnerLUID}
		return out, nil
	}
	result, err := a.updater.UpdateFlow(ctx, request)
	if err != nil {
		return Output{}, operationError("flow.update.failed", in, current.LUID, "Flow update failed.", "Inspect the exact flow before retrying: "+commandhint.Environment(in.Environment, "content", "flow", "inspect", "--id", current.LUID), err)
	}
	if result.FlowLUID == "" {
		result.FlowLUID = current.LUID
	}
	if result.FlowName == "" {
		result.FlowName = current.Name
	}
	if result.ProjectLUID == "" {
		result.ProjectLUID = current.ProjectLUID
	}
	if result.OwnerLUID == "" {
		result.OwnerLUID = *request.OwnerLUID
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "content", "flow", "inspect", "--id", current.LUID)}
	return out, nil
}
func changedRequest(target Flow, in Input) (Request, []Change) {
	r := Request{LUID: target.LUID}
	changes := []Change{}
	if in.OwnerLUID != nil && *in.OwnerLUID != target.OwnerLUID {
		r.OwnerLUID = in.OwnerLUID
		changes = append(changes, Change{Field: "owner_luid", Before: target.OwnerLUID, After: *in.OwnerLUID})
	}
	return r, changes
}
func validate(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return usage("environment", "flow update requires an explicit resolved environment and site")
	}
	return ValidateInput(in)
}
func usage(field, message string) error {
	return &errs.Error{ID: "flow.update.usage", Kind: errs.KindUsage, Operation: "flow.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func operationError(id string, in Input, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "flow.update", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
