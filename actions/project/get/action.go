package get

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned exact project seam.
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

// Action gets one exact project.
type Action struct{ resolver Resolver }

// New creates a project get action.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative project.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "project.get.unconfigured", Kind: errs.KindRuntime, Operation: "project.get", Summary: "Project get is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the project resolver before retrying."}
	}
	if input.Selector.LUID == "" && input.Selector.ProjectPath == "" {
		return Output{}, &errs.Error{ID: "project.get.usage", Kind: errs.KindUsage, Operation: "project.get", Summary: "project LUID or exact project path is required", Retryable: errs.Bool(false), CorrectiveAction: "Provide a project LUID or an exact project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "project LUID or exact project path is required"}}}
	}
	project, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
		return Output{}, &errs.Error{ID: "project.get.resolve", Kind: errs.KindOperation, Operation: "project.get", Environment: input.Environment, Site: input.Site, Summary: "Project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Project: project, RequestID: project.RequestID, Help: []string{"tadx content project list"}}, nil
}
