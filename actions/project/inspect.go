package project

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned exact project seam.
type InspectResolver interface {
	ResolveProject(context.Context, identity.Selector) (InspectProject, error)
}

// ValidateInput checks an exact selector before authentication.
func ValidateInspectInput(input InspectInput) error {
	if input.Selector.LUID == "" && input.Selector.ProjectPath == "" {
		return &errs.Error{ID: "project.inspect.usage", Kind: errs.KindUsage, Operation: "project.inspect", Summary: "project LUID or exact project path is required", Retryable: errs.Bool(false), CorrectiveAction: "Provide a project LUID or an exact project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "project LUID or exact project path is required"}}}
	}
	return nil
}

// inspectValidated resolves one authoritative project.
func (a *runner) inspectValidated(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if a == nil || a.InspectResolver == nil {
		return InspectOutput{}, &errs.Error{ID: "project.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "project.inspect", Summary: "Project inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the project resolver before retrying."}
	}
	project, err := a.InspectResolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return InspectOutput{}, err
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
		return InspectOutput{}, &errs.Error{ID: "project.inspect.resolve", Kind: errs.KindOperation, Operation: "project.inspect", Environment: input.Environment, Site: input.Site, Summary: "Project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, Project: project, RequestID: project.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", project.LUID, "--full")}}, nil
}
