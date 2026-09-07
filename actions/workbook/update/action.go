package update

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type Resolver interface {
	ResolveWorkbook(context.Context, identity.Selector) (Workbook, error)
	FindWorkbooks(context.Context, string, string) ([]Workbook, error)
}
type Updater interface {
	UpdateWorkbook(context.Context, Request) (Result, error)
}
type Action struct {
	resolver Resolver
	updater  Updater
}

func New(resolver Resolver, updater Updater) *Action {
	return &Action{resolver: resolver, updater: updater}
}

func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.updater == nil {
		return Output{}, &errs.Error{ID: "workbook.update.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.update", Summary: "Workbook update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook update before retrying."}
	}
	if err := validate(input); err != nil {
		return Output{}, err
	}
	target, err := a.resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		return Output{}, operationError("workbook.update.resolve", input, "", "Workbook resolution failed.", "Review the exact workbook selector, then retry.", err)
	}
	request, changes := changedRequest(target, input)
	if err := a.rejectCollision(ctx, input, target, request.Name); err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "workbook.update", Environment: input.Environment, Site: input.Site, Target: target, Changes: changes, NoOp: len(changes) == 0}, Help: []string{"Run without --preview to update this exact workbook."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.resolver.ResolveWorkbook(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return Output{}, operationError("workbook.update.resolve", input, target.LUID, "Workbook revalidation failed.", "Review a new preview before updating.", err)
	}
	if current.Name != target.Name || current.ProjectLUID != target.ProjectLUID || current.OwnerLUID != target.OwnerLUID {
		return Output{}, operationError("workbook.update.target_changed", input, target.LUID, "The workbook changed during revalidation.", "Review a new preview before updating.", errors.New("workbook identity changed during revalidation"))
	}
	request, changes = changedRequest(current, input)
	out.Plan.Target, out.Plan.Changes, out.Plan.NoOp = current, changes, len(changes) == 0
	if err := a.rejectCollision(ctx, input, current, request.Name); err != nil {
		return Output{}, err
	}
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", WorkbookLUID: current.LUID, WorkbookName: current.Name, ProjectLUID: current.ProjectLUID, OwnerLUID: current.OwnerLUID}
		return out, nil
	}
	result, err := a.updater.UpdateWorkbook(ctx, request)
	if err != nil {
		return Output{}, operationError("workbook.update.failed", input, current.LUID, "Workbook update failed.", "Inspect the workbook before updating again.", err)
	}
	out.Result = &result
	out.Help = []string{"tadx content workbook inspect --id " + current.LUID}
	return out, nil
}

func changedRequest(target Workbook, input Input) (Request, []Change) {
	r := Request{LUID: target.LUID}
	changes := []Change{}
	if input.Name != nil && *input.Name != target.Name {
		r.Name = input.Name
		changes = append(changes, Change{Field: "name", Before: target.Name, After: *input.Name})
	}
	if input.OwnerLUID != nil && *input.OwnerLUID != target.OwnerLUID {
		r.OwnerLUID = input.OwnerLUID
		changes = append(changes, Change{Field: "owner_luid", Before: target.OwnerLUID, After: *input.OwnerLUID})
	}
	return r, changes
}

func (a *Action) rejectCollision(ctx context.Context, input Input, target Workbook, name *string) error {
	if name == nil {
		return nil
	}
	matches, err := a.resolver.FindWorkbooks(ctx, *name, target.ProjectLUID)
	if err != nil {
		return operationError("workbook.update.collision", input, target.LUID, "Workbook collision check failed.", "Review the target project before updating.", err)
	}
	for _, match := range matches {
		if match.LUID != target.LUID {
			return operationError("workbook.update.collision", input, target.LUID, "The project already contains this workbook name.", "Choose another workbook name.", errors.New("exact workbook name collision in target project"))
		}
	}
	return nil
}

func validate(input Input) error {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return usage("environment", "workbook update requires an explicit resolved environment and site")
	}
	if input.Selector.LUID == "" && (strings.TrimSpace(input.Selector.Name) == "" || strings.TrimSpace(input.Selector.ProjectPath) == "") {
		return usage("selector", "workbook update requires a LUID or exact name and project path")
	}
	if input.Selector.LUID != "" && (strings.TrimSpace(input.Selector.Name) != "" || strings.TrimSpace(input.Selector.ProjectPath) != "") {
		return usage("selector", "a workbook LUID cannot be combined with name or project path")
	}
	if input.Name == nil && input.OwnerLUID == nil {
		return usage("changes", "workbook update requires a name or owner LUID")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return usage("name", "workbook update name cannot be empty")
	}
	if input.OwnerLUID != nil && strings.TrimSpace(*input.OwnerLUID) == "" {
		return usage("owner_id", "workbook update owner LUID cannot be empty")
	}
	return nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "workbook.update.usage", Kind: errs.KindUsage, Operation: "workbook.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the workbook update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func operationError(id string, input Input, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "workbook.update", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
