package update

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver owns exact project identity resolution.
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

// Updater performs one exact released project update request.
type Updater interface {
	UpdateProject(context.Context, UpdateRequest) (Result, error)
}

// Action updates bounded metadata on one exact project.
type Action struct {
	resolver Resolver
	updater  Updater
}

// New creates a project-update action.
func New(resolver Resolver, updater Updater) *Action {
	return &Action{resolver: resolver, updater: updater}
}

// Execute previews or updates one exact project.
func (a *Action) Execute(ctx context.Context, input Input, apply bool) (Output, error) {
	if a == nil || a.resolver == nil || a.updater == nil {
		return Output{}, &errs.Error{ID: "project.update.unconfigured", Kind: errs.KindRuntime, Operation: "project.update", Summary: "Project update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project update before retrying."}
	}
	if err := validateInput(input); err != nil {
		return Output{}, err
	}
	project, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		return Output{}, resolutionError(input, "Project resolution failed.", err)
	}
	request, noOp := changedRequest(project, input)
	plan := Plan{Mode: "preview", Operation: "project.update", Environment: input.Environment, Site: input.Site, Target: project, Changes: Changes{Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions}, NoOp: noOp}
	output := Output{Plan: plan, Help: []string{"Add --apply to update this exact project."}}
	if !apply {
		return output, nil
	}
	current, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		return Output{}, resolutionError(input, "Project revalidation failed.", err)
	}
	if current.LUID != project.LUID {
		return Output{}, &errs.Error{ID: "project.update.target_changed", Kind: errs.KindOperation, Operation: "project.update", Resource: project.LUID, Environment: input.Environment, Site: input.Site, Summary: "The project identity changed after preview.", Cause: errors.New("project LUID changed after preview"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before updating the project."}
	}
	request, noOp = changedRequest(current, input)
	output.Plan.Target = current
	output.Plan.NoOp = noOp
	if noOp {
		output.Applied = true
		output.Result = &Result{Status: "unchanged", Project: current}
		output.Help = []string{"tadx content project get --project-id " + current.LUID}
		return output, nil
	}
	result, err := a.updater.UpdateProject(ctx, request)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the remote project update outcome before retrying.")
		return Output{}, &errs.Error{ID: "project.update.failed", Kind: errs.KindOperation, Operation: "project.update", Resource: current.LUID, Environment: input.Environment, Site: input.Site, Summary: "Project update failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Applied = true
	output.Result = &result
	output.Help = []string{"tadx content project get --project-id " + current.LUID}
	return output, nil
}

func changedRequest(project Project, input Input) (UpdateRequest, bool) {
	request := UpdateRequest{LUID: project.LUID}
	if input.Name != nil && *input.Name != project.Name {
		request.Name = input.Name
	}
	if input.Description != nil && *input.Description != project.Description {
		request.Description = input.Description
	}
	if input.ContentPermissions != nil && *input.ContentPermissions != project.ContentPermissions {
		request.ContentPermissions = input.ContentPermissions
	}
	return request, request.Name == nil && request.Description == nil && request.ContentPermissions == nil
}

func validateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" || strings.TrimSpace(input.Site) == "" {
		return usage("environment", "project update requires an explicit resolved environment and site")
	}
	if input.Selector.LUID == "" && strings.TrimSpace(input.Selector.ProjectPath) == "" {
		return usage("selector", "project update requires a project LUID or exact project path")
	}
	if input.Selector.Name != "" || (input.Selector.LUID != "" && strings.TrimSpace(input.Selector.ProjectPath) != "") {
		return usage("selector", "use either a project LUID or an exact project path")
	}
	if input.Name == nil && input.Description == nil && input.ContentPermissions == nil {
		return usage("changes", "project update requires at least one explicit metadata change")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return usage("name", "project update name cannot be empty")
	}
	if input.Name != nil && strings.Contains(*input.Name, "/") {
		return usage("name", "project update name cannot contain a slash")
	}
	if input.ContentPermissions != nil && !validContentPermissions(*input.ContentPermissions) {
		return usage("content_permissions", "project update content permissions are invalid")
	}
	return nil
}

func validContentPermissions(value string) bool {
	return value == "ManagedByOwner" || value == "LockedToProject" || value == "LockedToProjectWithoutNested"
}

func resolutionError(input Input, summary string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
	return &errs.Error{ID: "project.update.resolve", Kind: errs.KindOperation, Operation: "project.update", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

func usage(field, message string) error {
	return &errs.Error{ID: "project.update.usage", Kind: errs.KindUsage, Operation: "project.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
