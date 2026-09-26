package flow

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type Updater interface {
	UpdateFlow(context.Context, UpdateRequest) (UpdateResult, error)
}

// Update previews or updates the owner of one exact flow.
func Update(ctx context.Context, resolver Resolver, updater Updater, in UpdateInput, preview bool) (UpdateOutput, error) {
	if resolver == nil || updater == nil {
		return UpdateOutput{}, &errs.Error{ID: "flow.update.unconfigured", Kind: errs.KindRuntime, Operation: "flow.update", Summary: "Flow update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow update before retrying."}
	}
	if err := updateValidate(in); err != nil {
		return UpdateOutput{}, err
	}
	target, err := resolver.ResolveFlow(ctx, in.Selector)
	if err != nil {
		return UpdateOutput{}, updateOperationError("flow.update.resolve", in, "", "Flow resolution failed.", "Review the exact flow selector, then retry.", err)
	}
	request, changes := updateChangedRequest(target, in)
	out := UpdateOutput{Plan: UpdatePlan{Mode: "preview", Operation: "flow.update", Environment: in.Environment, Site: in.Site, Target: updateIdentity(target), Changes: changes, NoOp: len(changes) == 0}, Help: []string{"Run without --preview to update this exact flow owner."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := resolver.ResolveFlow(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return UpdateOutput{}, updateOperationError("flow.update.resolve", in, target.LUID, "Flow revalidation failed.", "Review a new preview before updating.", err)
	}
	if current.Name != target.Name || current.ProjectLUID != target.ProjectLUID || current.OwnerLUID != target.OwnerLUID {
		return UpdateOutput{}, updateOperationError("flow.update.target_changed", in, target.LUID, "The flow changed during revalidation.", "Review a new preview before updating.", errors.New("flow identity changed during revalidation"))
	}
	request, changes = updateChangedRequest(current, in)
	out.Plan.Target, out.Plan.Changes, out.Plan.NoOp = updateIdentity(current), changes, len(changes) == 0
	if out.Plan.NoOp {
		out.Result = &UpdateResult{Status: "unchanged", FlowLUID: current.LUID, FlowName: current.Name, ProjectLUID: current.ProjectLUID, OwnerLUID: current.OwnerLUID}
		return out, nil
	}
	result, err := updater.UpdateFlow(ctx, request)
	if err != nil {
		return UpdateOutput{}, updateOperationError("flow.update.failed", in, current.LUID, "Flow update failed.", "Inspect the exact flow before retrying: "+commandhint.Environment(in.Environment, "content", "flow", "inspect", "--id", current.LUID), err)
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
func updateChangedRequest(target Record, in UpdateInput) (UpdateRequest, []UpdateChange) {
	r := UpdateRequest{LUID: target.LUID}
	changes := []UpdateChange{}
	if in.OwnerLUID != nil && *in.OwnerLUID != target.OwnerLUID {
		r.OwnerLUID = in.OwnerLUID
		changes = append(changes, UpdateChange{Field: "owner_luid", Before: target.OwnerLUID, After: *in.OwnerLUID})
	}
	return r, changes
}
func updateValidate(in UpdateInput) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return updateUsage("environment", "flow update requires an explicit resolved environment and site")
	}
	return ValidateUpdateInput(in)
}
func updateUsage(field, message string) error {
	return &errs.Error{ID: "flow.update.usage", Kind: errs.KindUsage, Operation: "flow.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func updateOperationError(id string, in UpdateInput, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "flow.update", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidateUpdateInput checks caller-controlled arguments before local or remote setup.
func ValidateUpdateInput(in UpdateInput) error {
	if strings.TrimSpace(in.Environment) == "" {
		return updateUsage("environment", "flow update requires an explicit environment")
	}
	if in.Selector.LUID == "" && (strings.TrimSpace(in.Selector.Name) == "" || strings.TrimSpace(in.Selector.ProjectPath) == "") {
		return updateUsage("selector", "flow update requires a LUID or exact name and project path")
	}
	if in.Selector.LUID != "" && (strings.TrimSpace(in.Selector.Name) != "" || strings.TrimSpace(in.Selector.ProjectPath) != "") {
		return updateUsage("selector", "a flow LUID cannot be combined with name or project path")
	}
	if in.OwnerLUID == nil {
		return updateUsage("owner_id", "flow update requires an exact owner LUID")
	}
	if err := identity.ValidateLUIDShape("owner", *in.OwnerLUID); err != nil {
		return updateUsage("owner_id", err.Error())
	}
	return nil
}
