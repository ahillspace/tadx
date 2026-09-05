package move

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type Resolver interface {
	ResolveWorkbook(context.Context, identity.Selector) (Workbook, error)
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindWorkbooks(context.Context, string, string) ([]Workbook, error)
}

type Mover interface {
	MoveWorkbook(context.Context, string, string) (Result, error)
}

type Action struct {
	resolver Resolver
	mover    Mover
}

func New(resolver Resolver, mover Mover) *Action { return &Action{resolver: resolver, mover: mover} }

func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.mover == nil {
		return Output{}, runtimeError()
	}
	if err := validate(input); err != nil {
		return Output{}, err
	}
	source, destination, err := a.resolve(ctx, input)
	if err != nil {
		return Output{}, err
	}
	if err := a.rejectCollision(ctx, input, source, destination); err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "workbook.move", Environment: input.Environment, Site: input.Site, Source: source, Destination: destination, NoOp: source.ProjectLUID == destination.LUID}, Help: []string{"Run without --preview to move this exact workbook."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, currentDestination, err := a.resolveByLUID(ctx, input, source.LUID, destination.LUID)
	if err != nil {
		return Output{}, err
	}
	if current.Name != source.Name || current.ProjectLUID != source.ProjectLUID || current.OwnerLUID != source.OwnerLUID || currentDestination.LUID != destination.LUID {
		return Output{}, operationError("workbook.move.target_changed", input, source.LUID, "The workbook move source or destination changed during revalidation.", "Review a new preview before moving.", errors.New("workbook move identity changed during revalidation"))
	}
	if err := a.rejectCollision(ctx, input, current, currentDestination); err != nil {
		return Output{}, err
	}
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", WorkbookLUID: source.LUID, ProjectLUID: destination.LUID}
		return out, nil
	}
	result, err := a.mover.MoveWorkbook(ctx, source.LUID, destination.LUID)
	if err != nil {
		return Output{}, operationError("workbook.move.failed", input, source.LUID, "Workbook move failed.", "Inspect the workbook before moving again.", err)
	}
	out.Result = &result
	out.Help = []string{"tadx content workbook inspect --id " + source.LUID}
	return out, nil
}

func (a *Action) resolve(ctx context.Context, input Input) (Workbook, Project, error) {
	source, err := a.resolver.ResolveWorkbook(ctx, input.WorkbookSelector)
	if err != nil {
		return Workbook{}, Project{}, operationError("workbook.move.resolve", input, "", "Workbook resolution failed.", "Review the exact workbook selector, then retry.", err)
	}
	destination, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		return Workbook{}, Project{}, operationError("workbook.move.project", input, source.LUID, "Destination project resolution failed.", "Review the exact destination project, then retry.", err)
	}
	return source, destination, nil
}

func (a *Action) resolveByLUID(ctx context.Context, input Input, workbookLUID, projectLUID string) (Workbook, Project, error) {
	copy := input
	copy.WorkbookSelector = identity.Selector{LUID: identity.LUID(workbookLUID)}
	copy.ProjectSelector = identity.Selector{LUID: identity.LUID(projectLUID)}
	return a.resolve(ctx, copy)
}

func (a *Action) rejectCollision(ctx context.Context, input Input, source Workbook, destination Project) error {
	if source.ProjectLUID == destination.LUID {
		return nil
	}
	matches, err := a.resolver.FindWorkbooks(ctx, source.Name, destination.LUID)
	if err != nil {
		return operationError("workbook.move.collision", input, source.LUID, "Workbook collision check failed.", "Review the exact destination before moving.", err)
	}
	for _, match := range matches {
		if match.LUID != source.LUID {
			return operationError("workbook.move.collision", input, source.LUID, "The destination already contains this workbook name.", "Rename the workbook or choose another destination.", errors.New("exact workbook name collision in destination project"))
		}
	}
	return nil
}

func validate(input Input) error {
	if strings.TrimSpace(input.Environment) == "" || strings.TrimSpace(input.Site) == "" {
		return usage("environment", "workbook move requires an explicit resolved environment and site")
	}
	if input.WorkbookSelector.LUID == "" && (strings.TrimSpace(input.WorkbookSelector.Name) == "" || strings.TrimSpace(input.WorkbookSelector.ProjectPath) == "") {
		return usage("selector", "workbook move requires a LUID or exact name and project path")
	}
	if input.WorkbookSelector.LUID != "" && (strings.TrimSpace(input.WorkbookSelector.Name) != "" || strings.TrimSpace(input.WorkbookSelector.ProjectPath) != "") {
		return usage("selector", "a workbook LUID cannot be combined with name or project path")
	}
	if input.ProjectSelector.LUID == "" && strings.TrimSpace(input.ProjectSelector.ProjectPath) == "" {
		return usage("project", "workbook move requires a destination project LUID or exact path")
	}
	if input.ProjectSelector.LUID != "" && strings.TrimSpace(input.ProjectSelector.ProjectPath) != "" {
		return usage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}

func runtimeError() error {
	return &errs.Error{ID: "workbook.move.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.move", Summary: "Workbook move is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook move before retrying."}
}

func usage(field, message string) error {
	return &errs.Error{ID: "workbook.move.usage", Kind: errs.KindUsage, Operation: "workbook.move", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the workbook move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func operationError(id string, input Input, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "workbook.move", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
