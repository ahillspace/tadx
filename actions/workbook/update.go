package workbook

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type UpdateResolver interface {
	Resolver
	CollisionReader
}
type Updater interface {
	UpdateWorkbook(context.Context, UpdateRequest) (UpdateResult, error)
}

func Update(ctx context.Context, resolver UpdateResolver, updater Updater, input UpdateInput, preview bool) (UpdateOutput, error) {
	if resolver == nil || updater == nil {
		return UpdateOutput{}, &errs.Error{ID: "workbook.update.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.update", Summary: "Workbook update is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook update before retrying."}
	}
	if err := updateValidate(input); err != nil {
		return UpdateOutput{}, err
	}
	target, err := resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		return UpdateOutput{}, updateOperationError("workbook.update.resolve", input, "", "Workbook resolution failed.", "Review the exact workbook selector, then retry.", err)
	}
	request, changes := updateChangedRequest(target, input)
	if err := updateRejectCollision(ctx, resolver, input, target, request.Name); err != nil {
		return UpdateOutput{}, err
	}
	out := UpdateOutput{Plan: UpdatePlan{Mode: "preview", Operation: "workbook.update", Environment: input.Environment, Site: input.Site, Target: updateIdentity(target), Changes: changes, NoOp: len(changes) == 0}, Help: []string{"Run without --preview to update this exact workbook."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := resolver.ResolveWorkbook(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return UpdateOutput{}, updateOperationError("workbook.update.resolve", input, target.LUID, "Workbook revalidation failed.", "Review a new preview before updating.", err)
	}
	if current.Name != target.Name || current.ProjectLUID != target.ProjectLUID || current.OwnerLUID != target.OwnerLUID || (input.Description != nil && current.Description != target.Description) {
		return UpdateOutput{}, updateOperationError("workbook.update.target_changed", input, target.LUID, "The workbook changed during revalidation.", "Review a new preview before updating.", errors.New("workbook identity changed during revalidation"))
	}
	request, changes = updateChangedRequest(current, input)
	out.Plan.Target, out.Plan.Changes, out.Plan.NoOp = updateIdentity(current), changes, len(changes) == 0
	if err := updateRejectCollision(ctx, resolver, input, current, request.Name); err != nil {
		return UpdateOutput{}, err
	}
	if out.Plan.NoOp {
		out.Result = &UpdateResult{Status: "unchanged", WorkbookLUID: current.LUID, WorkbookName: current.Name, ProjectLUID: current.ProjectLUID, OwnerLUID: current.OwnerLUID}
		return out, nil
	}
	result, err := updater.UpdateWorkbook(ctx, request)
	out.Result = &result
	if err != nil {
		return out, updateOperationError("workbook.update.failed", input, current.LUID, "Workbook update failed.", "Inspect the exact workbook before retrying: "+commandhint.Environment(input.Environment, "content", "workbook", "inspect", "--id", current.LUID), err)
	}
	out.Help = []string{commandhint.Environment(input.Environment, "content", "workbook", "inspect", "--id", current.LUID)}
	return out, nil
}

func updateChangedRequest(target Record, input UpdateInput) (UpdateRequest, []UpdateChange) {
	r := UpdateRequest{LUID: target.LUID}
	changes := []UpdateChange{}
	if input.Description != nil && *input.Description != target.Description {
		r.Description = input.Description
		changes = append(changes, UpdateChange{Field: "description", Before: target.Description, After: *input.Description})
	}
	if input.Name != nil && *input.Name != target.Name {
		r.Name = input.Name
		changes = append(changes, UpdateChange{Field: "name", Before: target.Name, After: *input.Name})
	}
	if input.OwnerLUID != nil && *input.OwnerLUID != target.OwnerLUID {
		r.OwnerLUID = input.OwnerLUID
		changes = append(changes, UpdateChange{Field: "owner_luid", Before: target.OwnerLUID, After: *input.OwnerLUID})
	}
	return r, changes
}

func updateRejectCollision(ctx context.Context, resolver UpdateResolver, input UpdateInput, target Record, name *string) error {
	if name == nil {
		return nil
	}
	matches, err := resolver.FindWorkbooks(ctx, *name, target.ProjectLUID)
	if err != nil {
		return updateOperationError("workbook.update.collision", input, target.LUID, "Workbook collision check failed.", "Review the target project before updating.", err)
	}
	for _, match := range matches {
		if match.LUID != target.LUID {
			return updateOperationError("workbook.update.collision", input, target.LUID, "The project already contains this workbook name.", "Choose another workbook name.", errors.New("exact workbook name collision in target project"))
		}
	}
	return nil
}

func updateValidate(input UpdateInput) error {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return updateUsage("environment", "workbook update requires an explicit resolved environment and site")
	}
	return ValidateUpdateInput(input)
}

func updateUsage(field, message string) error {
	return &errs.Error{ID: "workbook.update.usage", Kind: errs.KindUsage, Operation: "workbook.update", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the workbook update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func updateOperationError(id string, input UpdateInput, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "workbook.update", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidateUpdateInput checks caller-controlled arguments before local or remote setup.
func ValidateUpdateInput(input UpdateInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return updateUsage("environment", "workbook update requires an explicit environment")
	}
	if input.Selector.LUID == "" && (strings.TrimSpace(input.Selector.Name) == "" || strings.TrimSpace(input.Selector.ProjectPath) == "") {
		return updateUsage("selector", "workbook update requires a LUID or exact name and project path")
	}
	if input.Selector.LUID != "" && (strings.TrimSpace(input.Selector.Name) != "" || strings.TrimSpace(input.Selector.ProjectPath) != "") {
		return updateUsage("selector", "a workbook LUID cannot be combined with name or project path")
	}
	if input.Name == nil && input.OwnerLUID == nil && input.Description == nil {
		return updateUsage("changes", "workbook update requires a name, owner LUID, or description")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return updateUsage("name", "workbook update name cannot be empty")
	}
	if input.OwnerLUID != nil {
		if err := identity.ValidateLUIDShape("owner", *input.OwnerLUID); err != nil {
			return updateUsage("owner_id", err.Error())
		}
	}
	return nil
}
