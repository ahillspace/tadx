package move

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

type Resolver interface {
	ResolveDatasource(context.Context, identity.Selector) (Datasource, error)
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindDatasources(context.Context, string, string) ([]Datasource, error)
}
type Mover interface {
	MoveDatasource(context.Context, string, string) (Result, error)
}
type Action struct {
	resolver Resolver
	mover    Mover
}

func New(r Resolver, m Mover) *Action { return &Action{r, m} }
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	ctx = a.beginProjectResolution(ctx)
	if a == nil || a.resolver == nil || a.mover == nil {
		return Output{}, runtimeError()
	}
	if err := validate(in); err != nil {
		return Output{}, err
	}
	source, destination, err := a.resolve(ctx, in, in.DatasourceSelector, in.ProjectSelector)
	if err != nil {
		return Output{}, err
	}
	if err = a.collision(ctx, in, source, destination); err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "datasource.move", Environment: in.Environment, Site: in.Site, Source: source, Destination: destination, NoOp: source.ProjectLUID == destination.LUID}, Help: []string{"Run without --preview to move this exact datasource."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	ctx = a.beginProjectResolution(ctx)
	current, currentDestination, err := a.resolve(ctx, in, identity.Selector{LUID: identity.LUID(source.LUID)}, identity.Selector{LUID: identity.LUID(destination.LUID)})
	if err != nil {
		return Output{}, err
	}
	if current.Name != source.Name || current.ProjectLUID != source.ProjectLUID || current.OwnerLUID != source.OwnerLUID || currentDestination.LUID != destination.LUID {
		return Output{}, operationError("datasource.move.target_changed", in, source.LUID, "The datasource move source or destination changed during revalidation.", "Review a new preview before moving.", errors.New("datasource move identity changed during revalidation"))
	}
	if err = a.collision(ctx, in, current, currentDestination); err != nil {
		return Output{}, err
	}
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", DatasourceLUID: source.LUID, ProjectLUID: destination.LUID}
		return out, nil
	}
	result, err := a.mover.MoveDatasource(ctx, source.LUID, destination.LUID)
	if err != nil {
		return Output{}, operationError("datasource.move.failed", in, source.LUID, "Datasource move failed.", "Inspect the datasource before moving again.", err)
	}
	out.Result = &result
	out.Help = []string{"tadx content datasource inspect --id " + source.LUID}
	return out, nil
}
func (a *Action) resolve(ctx context.Context, in Input, sourceSelector, projectSelector identity.Selector) (Datasource, Project, error) {
	source, err := a.resolver.ResolveDatasource(ctx, sourceSelector)
	if err != nil {
		return Datasource{}, Project{}, operationError("datasource.move.resolve", in, "", "Datasource resolution failed.", "Review the exact datasource selector, then retry.", err)
	}
	destination, err := a.resolver.ResolveProject(ctx, projectSelector)
	if err != nil {
		return Datasource{}, Project{}, operationError("datasource.move.project", in, source.LUID, "Destination project resolution failed.", "Review the exact destination project, then retry.", err)
	}
	return source, destination, nil
}
func (a *Action) collision(ctx context.Context, in Input, source Datasource, destination Project) error {
	if source.ProjectLUID == destination.LUID {
		return nil
	}
	matches, err := a.resolver.FindDatasources(ctx, source.Name, destination.LUID)
	if err != nil {
		return operationError("datasource.move.collision", in, source.LUID, "Datasource collision check failed.", "Review the exact destination before moving.", err)
	}
	for _, match := range matches {
		if match.LUID != source.LUID {
			return operationError("datasource.move.collision", in, source.LUID, "The destination already contains this datasource name.", "Rename the datasource or choose another destination.", errors.New("exact datasource name collision in destination project"))
		}
	}
	return nil
}
func validate(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return usage("environment", "datasource move requires an explicit resolved environment and site")
	}
	if in.DatasourceSelector.LUID == "" && (strings.TrimSpace(in.DatasourceSelector.Name) == "" || strings.TrimSpace(in.DatasourceSelector.ProjectPath) == "") {
		return usage("selector", "datasource move requires a LUID or exact name and project path")
	}
	if in.DatasourceSelector.LUID != "" && (strings.TrimSpace(in.DatasourceSelector.Name) != "" || strings.TrimSpace(in.DatasourceSelector.ProjectPath) != "") {
		return usage("selector", "a datasource LUID cannot be combined with name or project path")
	}
	if in.ProjectSelector.LUID == "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) == "" {
		return usage("project", "datasource move requires a destination project LUID or exact path")
	}
	if in.ProjectSelector.LUID != "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) != "" {
		return usage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
func runtimeError() error {
	return &errs.Error{ID: "datasource.move.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.move", Summary: "Datasource move is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure datasource move before retrying."}
}
func usage(field, message string) error {
	return &errs.Error{ID: "datasource.move.usage", Kind: errs.KindUsage, Operation: "datasource.move", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the datasource move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func operationError(id string, in Input, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.move", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
