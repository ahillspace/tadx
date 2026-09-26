package datasource

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

type MoveResolver interface {
	Resolver
	ProjectResolver
	CollisionReader
}
type Mover interface {
	MoveDatasource(context.Context, string, string) (MoveResult, error)
}

func Move(ctx context.Context, resolver MoveResolver, mover Mover, in MoveInput, preview bool) (MoveOutput, error) {
	ctx = beginProjectResolution(ctx, resolver)
	if resolver == nil || mover == nil {
		return MoveOutput{}, moveRuntimeError()
	}
	if err := moveValidate(in); err != nil {
		return MoveOutput{}, err
	}
	source, destination, err := moveResolve(ctx, resolver, in, in.DatasourceSelector, in.ProjectSelector)
	if err != nil {
		return MoveOutput{}, err
	}
	if err = moveCollision(ctx, resolver, in, source, destination); err != nil {
		return MoveOutput{}, err
	}
	out := MoveOutput{Plan: MovePlan{Mode: "preview", Operation: "datasource.move", Environment: in.Environment, Site: in.Site, Source: moveIdentity(source), Destination: destination, NoOp: source.ProjectLUID == destination.LUID}, Help: []string{"Run without --preview to move this exact datasource."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	ctx = beginProjectResolution(ctx, resolver)
	current, currentDestination, err := moveResolve(ctx, resolver, in, identity.Selector{LUID: identity.LUID(source.LUID)}, identity.Selector{LUID: identity.LUID(destination.LUID)})
	if err != nil {
		return MoveOutput{}, err
	}
	if current.Name != source.Name || current.ProjectLUID != source.ProjectLUID || current.OwnerLUID != source.OwnerLUID || currentDestination.LUID != destination.LUID {
		return MoveOutput{}, moveOperationError("datasource.move.target_changed", in, source.LUID, "The datasource move source or destination changed during revalidation.", "Review a new preview before moving.", errors.New("datasource move identity changed during revalidation"))
	}
	if err = moveCollision(ctx, resolver, in, current, currentDestination); err != nil {
		return MoveOutput{}, err
	}
	if out.Plan.NoOp {
		out.Result = &MoveResult{Status: "unchanged", DatasourceLUID: source.LUID, ProjectLUID: destination.LUID}
		return out, nil
	}
	result, err := mover.MoveDatasource(ctx, source.LUID, destination.LUID)
	if err != nil {
		return MoveOutput{}, moveOperationError("datasource.move.failed", in, source.LUID, "Datasource move failed.", "Inspect the exact datasource before retrying: "+commandhint.Environment(in.Environment, "content", "datasource", "inspect", "--id", source.LUID), err)
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "inspect", "--id", source.LUID)}
	return out, nil
}
func moveResolve(ctx context.Context, resolver MoveResolver, in MoveInput, sourceSelector, projectSelector identity.Selector) (Record, Project, error) {
	source, err := resolver.ResolveDatasource(ctx, sourceSelector)
	if err != nil {
		return Record{}, Project{}, moveOperationError("datasource.move.resolve", in, "", "Datasource resolution failed.", "Review the exact datasource selector, then retry.", err)
	}
	destination, err := resolver.ResolveProject(ctx, projectSelector)
	if err != nil {
		return Record{}, Project{}, moveOperationError("datasource.move.project", in, source.LUID, "Destination project resolution failed.", "Review the exact destination project, then retry.", err)
	}
	return source, destination, nil
}
func moveCollision(ctx context.Context, resolver MoveResolver, in MoveInput, source Record, destination Project) error {
	if source.ProjectLUID == destination.LUID {
		return nil
	}
	matches, err := resolver.FindDatasources(ctx, source.Name, destination.LUID)
	if err != nil {
		return moveOperationError("datasource.move.collision", in, source.LUID, "Datasource collision check failed.", "Review the exact destination before moving.", err)
	}
	for _, match := range matches {
		if match.LUID != source.LUID {
			return moveOperationError("datasource.move.collision", in, source.LUID, "The destination already contains this datasource name.", "Rename the datasource or choose another destination.", errors.New("exact datasource name collision in destination project"))
		}
	}
	return nil
}
func moveValidate(in MoveInput) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return moveUsage("environment", "datasource move requires an explicit resolved environment and site")
	}
	return ValidateMoveInput(in)
}
func moveRuntimeError() error {
	return &errs.Error{ID: "datasource.move.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.move", Summary: "Datasource move is not configured.", Retryable: new(false), CorrectiveAction: "Configure datasource move before retrying."}
}
func moveUsage(field, message string) error {
	return &errs.Error{ID: "datasource.move.usage", Kind: errs.KindUsage, Operation: "datasource.move", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the datasource move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func moveOperationError(id string, in MoveInput, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.move", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidateMoveInput checks caller-controlled arguments before local or remote setup.
func ValidateMoveInput(in MoveInput) error {
	if strings.TrimSpace(in.Environment) == "" {
		return moveUsage("environment", "datasource move requires an explicit environment")
	}
	if in.DatasourceSelector.LUID == "" && (strings.TrimSpace(in.DatasourceSelector.Name) == "" || strings.TrimSpace(in.DatasourceSelector.ProjectPath) == "") {
		return moveUsage("selector", "datasource move requires a LUID or exact name and project path")
	}
	if in.DatasourceSelector.LUID != "" && (strings.TrimSpace(in.DatasourceSelector.Name) != "" || strings.TrimSpace(in.DatasourceSelector.ProjectPath) != "") {
		return moveUsage("selector", "a datasource LUID cannot be combined with name or project path")
	}
	if in.ProjectSelector.LUID == "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) == "" {
		return moveUsage("project", "datasource move requires a destination project LUID or exact path")
	}
	if in.ProjectSelector.LUID != "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) != "" {
		return moveUsage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
