package project

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver owns exact project identity resolution.
type UpdateResolver interface {
	ResolveProject(context.Context, identity.Selector) (UpdateProject, error)
}

// Updater performs one exact released project update request.
type Updater interface {
	UpdateProject(context.Context, UpdateRequest) (UpdateResult, error)
}

// updateValidated changes bounded metadata on one exact project.
func (a *runner) updateValidated(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if a == nil || a.UpdateResolver == nil || a.Updater == nil {
		return UpdateOutput{}, &errs.Error{ID: "project.update.unconfigured", Kind: errs.KindRuntime, Operation: "project.update", Summary: "Project update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project update before retrying."}
	}
	if err := updateValidateResolvedTarget(input); err != nil {
		return UpdateOutput{}, err
	}
	project, err := a.UpdateResolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		return UpdateOutput{}, updateResolutionError(input, "Project resolution failed.", err)
	}
	request, noOp := updateChangedRequest(project, input)
	plan := UpdatePlan{Mode: "preview", Operation: "project.update", Environment: input.Environment, Site: input.Site, Target: project, Changes: UpdateChanges{Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions}, NoOp: noOp}
	output := UpdateOutput{Plan: plan, Help: []string{"Run without --preview to update this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.UpdateResolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		return UpdateOutput{}, updateResolutionError(input, "Project revalidation failed.", err)
	}
	if current.LUID != project.LUID {
		return UpdateOutput{}, &errs.Error{ID: "project.update.target_changed", Kind: errs.KindOperation, Operation: "project.update", Resource: project.LUID, Environment: input.Environment, Site: input.Site, Summary: "The project identity changed during revalidation.", Cause: errors.New("project LUID changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before updating the project."}
	}
	request, noOp = updateChangedRequest(current, input)
	output.Plan.Target = current
	output.Plan.NoOp = noOp
	if noOp {
		output.Result = &UpdateResult{Status: "unchanged", Project: current}
		output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", current.LUID)}
		return output, nil
	}
	result, err := a.Updater.UpdateProject(ctx, request)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the remote project update outcome before retrying: "+commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", current.LUID))
		return UpdateOutput{}, &errs.Error{ID: "project.update.failed", Kind: errs.KindOperation, Operation: "project.update", Resource: current.LUID, Environment: input.Environment, Site: input.Site, Summary: "Project update failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", current.LUID)}
	return output, nil
}

func updateChangedRequest(project UpdateProject, input UpdateInput) (UpdateRequest, bool) {
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

func updateValidateResolvedTarget(input UpdateInput) error {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return updateUsage("environment", "project update requires an explicit resolved environment and site")
	}
	return nil
}

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateUpdateInput(input UpdateInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return updateUsage("environment", "project update requires an explicit environment")
	}
	if input.Selector.LUID == "" && strings.TrimSpace(input.Selector.ProjectPath) == "" {
		return updateUsage("selector", "project update requires a project LUID or exact project path")
	}
	if input.Selector.Name != "" || (input.Selector.LUID != "" && strings.TrimSpace(input.Selector.ProjectPath) != "") {
		return updateUsage("selector", "use either a project LUID or an exact project path")
	}
	if input.Name == nil && input.Description == nil && input.ContentPermissions == nil {
		return updateUsage("changes", "project update requires at least one explicit metadata change")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return updateUsage("name", "project update name cannot be empty")
	}
	if input.Name != nil && strings.Contains(*input.Name, "/") {
		return updateUsage("name", "project update name cannot contain a slash")
	}
	if input.ContentPermissions != nil && !updateValidContentPermissions(*input.ContentPermissions) {
		return updateUsage("content_permissions", "project update content permissions are invalid")
	}
	return nil
}

func updateValidContentPermissions(value string) bool {
	return value == "ManagedByOwner" || value == "LockedToProject" || value == "LockedToProjectWithoutNested"
}

func updateResolutionError(input UpdateInput, summary string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
	return &errs.Error{ID: "project.update.resolve", Kind: errs.KindOperation, Operation: "project.update", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

func updateUsage(field, message string) error {
	return &errs.Error{ID: "project.update.usage", Kind: errs.KindUsage, Operation: "project.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
