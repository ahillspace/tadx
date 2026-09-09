package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned exact project seam.
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

// Action inspects one exact project.
type Action struct{ resolver Resolver }

// New creates a project inspect action.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative project.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "project.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "project.inspect", Summary: "Project inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the project resolver before retrying."}
	}
	var validationErr error
	input, validationErr = normalizeInput(input)
	if validationErr != nil {
		return Output{}, validationErr
	}
	project, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
		return Output{}, &errs.Error{ID: "project.inspect.resolve", Kind: errs.KindOperation, Operation: "project.inspect", Environment: input.Environment, Site: input.Site, Summary: "Project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Project: project, RequestID: project.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", project.LUID, "--full")}}, nil
}
