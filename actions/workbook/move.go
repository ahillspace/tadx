package workbook

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type MoveResolver interface {
	Resolver
	ProjectResolver
	CollisionReader
}

type Mover interface {
	MoveWorkbook(context.Context, string, string) (MoveResult, error)
}

func Move(ctx context.Context, resolver MoveResolver, mover Mover, input MoveInput, preview bool) (MoveOutput, error) {
	ctx = beginProjectResolution(ctx, resolver)
	if resolver == nil || mover == nil {
		return MoveOutput{}, moveRuntimeError()
	}
	if err := moveValidate(input); err != nil {
		return MoveOutput{}, err
	}
	source, destination, err := moveResolve(ctx, resolver, input)
	if err != nil {
		return MoveOutput{}, err
	}
	if err := moveRejectCollision(ctx, resolver, input, source, destination); err != nil {
		return MoveOutput{}, err
	}
	out := MoveOutput{Plan: MovePlan{Mode: "preview", Operation: "workbook.move", Environment: input.Environment, Site: input.Site, Source: moveIdentity(source), Destination: destination, NoOp: source.ProjectLUID == destination.LUID}, Help: []string{"Run without --preview to move this exact workbook."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	ctx = beginProjectResolution(ctx, resolver)
	current, currentDestination, err := moveResolveByLUID(ctx, resolver, input, source.LUID, destination.LUID)
	if err != nil {
		return MoveOutput{}, err
	}
	if current.Name != source.Name || current.ProjectLUID != source.ProjectLUID || current.OwnerLUID != source.OwnerLUID || currentDestination.LUID != destination.LUID {
		return MoveOutput{}, moveOperationError("workbook.move.target_changed", input, source.LUID, "The workbook move source or destination changed during revalidation.", "Review a new preview before moving.", errors.New("workbook move identity changed during revalidation"))
	}
	if err := moveRejectCollision(ctx, resolver, input, current, currentDestination); err != nil {
		return MoveOutput{}, err
	}
	if out.Plan.NoOp {
		out.Result = &MoveResult{Status: "unchanged", WorkbookLUID: source.LUID, ProjectLUID: destination.LUID}
		return out, nil
	}
	result, err := mover.MoveWorkbook(ctx, source.LUID, destination.LUID)
	if err != nil {
		return MoveOutput{}, moveOperationError("workbook.move.failed", input, source.LUID, "Workbook move failed.", "Inspect the exact workbook before retrying: "+commandhint.Environment(input.Environment, "content", "workbook", "inspect", "--id", source.LUID), err)
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(input.Environment, "content", "workbook", "inspect", "--id", source.LUID)}
	return out, nil
}

func moveResolve(ctx context.Context, resolver MoveResolver, input MoveInput) (Record, Project, error) {
	source, err := resolver.ResolveWorkbook(ctx, input.WorkbookSelector)
	if err != nil {
		return Record{}, Project{}, moveOperationError("workbook.move.resolve", input, "", "Workbook resolution failed.", "Review the exact workbook selector, then retry.", err)
	}
	destination, err := resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		return Record{}, Project{}, moveOperationError("workbook.move.project", input, source.LUID, "Destination project resolution failed.", "Review the exact destination project, then retry.", err)
	}
	return source, destination, nil
}

func moveResolveByLUID(ctx context.Context, resolver MoveResolver, input MoveInput, workbookLUID, projectLUID string) (Record, Project, error) {
	copy := input
	copy.WorkbookSelector = identity.Selector{LUID: identity.LUID(workbookLUID)}
	copy.ProjectSelector = identity.Selector{LUID: identity.LUID(projectLUID)}
	return moveResolve(ctx, resolver, copy)
}

func moveRejectCollision(ctx context.Context, resolver MoveResolver, input MoveInput, source Record, destination Project) error {
	if source.ProjectLUID == destination.LUID {
		return nil
	}
	matches, err := resolver.FindWorkbooks(ctx, source.Name, destination.LUID)
	if err != nil {
		return moveOperationError("workbook.move.collision", input, source.LUID, "Workbook collision check failed.", "Review the exact destination before moving.", err)
	}
	for _, match := range matches {
		if match.LUID != source.LUID {
			return moveOperationError("workbook.move.collision", input, source.LUID, "The destination already contains this workbook name.", "Rename the workbook or choose another destination.", errors.New("exact workbook name collision in destination project"))
		}
	}
	return nil
}

func moveValidate(input MoveInput) error {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return moveUsage("environment", "workbook move requires an explicit resolved environment and site")
	}
	return ValidateMoveInput(input)
}

func moveRuntimeError() error {
	return &errs.Error{ID: "workbook.move.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.move", Summary: "Workbook move is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook move before retrying."}
}

func moveUsage(field, message string) error {
	return &errs.Error{ID: "workbook.move.usage", Kind: errs.KindUsage, Operation: "workbook.move", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the workbook move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func moveOperationError(id string, input MoveInput, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "workbook.move", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidateMoveInput checks caller-controlled arguments before local or remote setup.
func ValidateMoveInput(input MoveInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return moveUsage("environment", "workbook move requires an explicit environment")
	}
	if input.WorkbookSelector.LUID == "" && (strings.TrimSpace(input.WorkbookSelector.Name) == "" || strings.TrimSpace(input.WorkbookSelector.ProjectPath) == "") {
		return moveUsage("selector", "workbook move requires a LUID or exact name and project path")
	}
	if input.WorkbookSelector.LUID != "" && (strings.TrimSpace(input.WorkbookSelector.Name) != "" || strings.TrimSpace(input.WorkbookSelector.ProjectPath) != "") {
		return moveUsage("selector", "a workbook LUID cannot be combined with name or project path")
	}
	if input.ProjectSelector.LUID == "" && strings.TrimSpace(input.ProjectSelector.ProjectPath) == "" {
		return moveUsage("project", "workbook move requires a destination project LUID or exact path")
	}
	if input.ProjectSelector.LUID != "" && strings.TrimSpace(input.ProjectSelector.ProjectPath) != "" {
		return moveUsage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
