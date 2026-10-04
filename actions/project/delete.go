package project

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver resolves an authoritative project LUID.
type DeleteResolver interface {
	ResolveProject(context.Context, identity.Selector) (DeleteProject, error)
}

// Deleter performs one exact remote project deletion.
type Deleter interface {
	DeleteProject(context.Context, string) (DeleteResult, error)
}

// deleteValidated previews and applies one exact project deletion.
func (a *runner) deleteValidated(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	if a == nil || a.DeleteResolver == nil || a.Deleter == nil {
		return DeleteOutput{}, deleteRuntimeError()
	}
	input.Environment = strings.TrimSpace(input.Environment)
	input.Site = strings.TrimSpace(input.Site)
	input.ProjectLUID = strings.TrimSpace(input.ProjectLUID)
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return DeleteOutput{}, deleteUsage("environment", "project delete requires an explicit resolved environment and site")
	}
	target, err := a.resolve(ctx, input, "Project resolution failed.", "Review the exact project LUID, then retry.")
	if err != nil {
		return DeleteOutput{}, err
	}
	plan := DeletePlan{Mode: "preview", Operation: "project.delete", Environment: input.Environment, Site: input.Site, Target: target}
	output := DeleteOutput{Plan: plan, Warnings: []string{DeleteCascadeWarning}, Help: []string{"Run without --preview to delete this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.resolve(ctx, input, "Project revalidation failed.", "Review a new preview before deleting.")
	if err != nil {
		return DeleteOutput{}, err
	}
	if current.LUID != target.LUID {
		return DeleteOutput{}, deleteOperationError("project.delete.target_changed", input, target.LUID, "The project identity changed during revalidation.", "Review a new preview before deleting.", errors.New("project LUID changed during revalidation"))
	}
	result, err := a.Deleter.DeleteProject(ctx, target.LUID)
	if err != nil {
		return DeleteOutput{}, deleteOperationError("project.delete.failed", input, target.LUID, "Project delete failed.", "Inspect the remote project delete outcome before retrying: "+commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", target.LUID), err)
	}
	if result.ProjectLUID != target.LUID {
		return DeleteOutput{}, deleteOperationError("project.delete.invalid_response", input, target.LUID, "Project delete returned a different authoritative identity.", "Review the remote project state before retrying.", errors.New("project delete result LUID did not match the requested LUID"))
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "list")}
	return output, nil
}

func (a *runner) resolve(ctx context.Context, input DeleteInput, summary, correctiveAction string) (DeleteProject, error) {
	project, err := a.DeleteResolver.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(input.ProjectLUID)})
	if err != nil {
		return DeleteProject{}, deleteOperationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, err)
	}
	if strings.TrimSpace(project.LUID) == "" {
		return DeleteProject{}, deleteOperationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, errors.New("resolved project omitted its authoritative LUID"))
	}
	if project.LUID != input.ProjectLUID {
		return DeleteProject{}, deleteOperationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, errors.New("resolved project LUID did not match the requested LUID"))
	}
	return project, nil
}

func deleteRuntimeError() error {
	return &errs.Error{ID: "project.delete.unconfigured", Kind: errs.KindRuntime, Operation: "project.delete", Summary: "Project delete is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project delete before retrying."}
}

// ValidateInput checks caller-controlled arguments before dependency setup.
func ValidateDeleteInput(input DeleteInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return deleteUsage("environment", "project delete requires an explicit environment")
	}
	if strings.TrimSpace(input.ProjectLUID) == "" {
		return deleteUsage("project_id", "project delete requires an authoritative project LUID")
	}
	return nil
}

func deleteUsage(field, message string) error {
	return &errs.Error{ID: "project.delete.usage", Kind: errs.KindUsage, Operation: "project.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact project LUID and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func deleteOperationError(id string, input DeleteInput, resource, summary, fallback string, cause error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "project.delete", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(cause)}
}
