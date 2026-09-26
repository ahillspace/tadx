package delete

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver resolves an authoritative project LUID.
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

// Deleter performs one exact remote project deletion.
type Deleter interface {
	DeleteProject(context.Context, string) (Result, error)
}

// Action previews and applies one exact project deletion.
type Action struct {
	resolver Resolver
	deleter  Deleter
}

// New creates a project-delete action.
func New(resolver Resolver, deleter Deleter) *Action {
	return &Action{resolver: resolver, deleter: deleter}
}

// Execute previews a deletion or revalidates its exact LUID before applying it.
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, runtimeError()
	}
	input.Environment = strings.TrimSpace(input.Environment)
	input.Site = strings.TrimSpace(input.Site)
	input.ProjectLUID = strings.TrimSpace(input.ProjectLUID)
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return Output{}, usage("environment", "project delete requires an explicit resolved environment and site")
	}
	target, err := a.resolve(ctx, input, "Project resolution failed.", "Review the exact project LUID, then retry.")
	if err != nil {
		return Output{}, err
	}
	plan := Plan{Mode: "preview", Operation: "project.delete", Environment: input.Environment, Site: input.Site, Target: target}
	output := Output{Plan: plan, Warnings: []string{CascadeWarning}, Help: []string{"Run without --preview to delete this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.resolve(ctx, input, "Project revalidation failed.", "Review a new preview before deleting.")
	if err != nil {
		return Output{}, err
	}
	if current.LUID != target.LUID {
		return Output{}, operationError("project.delete.target_changed", input, target.LUID, "The project identity changed during revalidation.", "Review a new preview before deleting.", errors.New("project LUID changed during revalidation"))
	}
	result, err := a.deleter.DeleteProject(ctx, target.LUID)
	if err != nil {
		return Output{}, operationError("project.delete.failed", input, target.LUID, "Project delete failed.", "Inspect the remote project delete outcome before retrying: "+commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", target.LUID), err)
	}
	if result.ProjectLUID != target.LUID {
		return Output{}, operationError("project.delete.invalid_response", input, target.LUID, "Project delete returned a different authoritative identity.", "Review the remote project state before retrying.", errors.New("project delete result LUID did not match the requested LUID"))
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "list")}
	return output, nil
}

func (a *Action) resolve(ctx context.Context, input Input, summary, correctiveAction string) (Project, error) {
	project, err := a.resolver.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(input.ProjectLUID)})
	if err != nil {
		return Project{}, operationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, err)
	}
	if strings.TrimSpace(project.LUID) == "" {
		return Project{}, operationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, errors.New("resolved project omitted its authoritative LUID"))
	}
	if project.LUID != input.ProjectLUID {
		return Project{}, operationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, errors.New("resolved project LUID did not match the requested LUID"))
	}
	return project, nil
}

func runtimeError() error {
	return &errs.Error{ID: "project.delete.unconfigured", Kind: errs.KindRuntime, Operation: "project.delete", Summary: "Project delete is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project delete before retrying."}
}

// ValidateInput checks caller-controlled arguments before dependency setup.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" {
		return usage("environment", "project delete requires an explicit environment")
	}
	if strings.TrimSpace(input.ProjectLUID) == "" {
		return usage("project_id", "project delete requires an authoritative project LUID")
	}
	return nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "project.delete.usage", Kind: errs.KindUsage, Operation: "project.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact project LUID and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func operationError(id string, input Input, resource, summary, fallback string, cause error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "project.delete", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(cause)}
}
