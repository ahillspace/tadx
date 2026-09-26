package move

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindProjectCollisions(context.Context, string, string) ([]Project, error)
}
type Mover interface {
	MoveProject(context.Context, string, *string) (Result, error)
}
type Action struct {
	resolver Resolver
	mover    Mover
}

func New(r Resolver, m Mover) *Action { return &Action{r, m} }

// A resolver may share project reads within one explicit validation phase.
// Each prewrite phase starts again, never inheriting the planning snapshot.
type projectResolutionPhase interface {
	BeginProjectResolution(context.Context) context.Context
}

func (a *Action) beginProjectResolution(ctx context.Context) context.Context {
	if a != nil {
		if resolver, ok := a.resolver.(projectResolutionPhase); ok {
			return resolver.BeginProjectResolution(ctx)
		}
	}
	return ctx
}

func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	ctx = a.beginProjectResolution(ctx)
	if a == nil || a.resolver == nil || a.mover == nil {
		return Output{}, &errs.Error{ID: "project.move.unconfigured", Kind: errs.KindRuntime, Operation: "project.move", Summary: "Project move is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project move before retrying."}
	}
	if err := validate(in); err != nil {
		return Output{}, err
	}
	source, err := a.resolver.ResolveProject(ctx, in.ProjectSelector)
	if err != nil {
		return Output{}, operationError("project.move.resolve", in, "", "Project resolution failed.", "Review the exact project selector, then retry.", err)
	}
	destination, parentLUID, err := a.destination(ctx, in, source)
	if err != nil {
		return Output{}, err
	}
	if err = a.validateMove(ctx, in, source, destination, parentLUID); err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "project.move", Environment: in.Environment, Site: in.Site, Source: source, Destination: destination, TopLevel: in.TopLevel, NoOp: source.ParentLUID == *parentLUID}, Help: []string{"Run without --preview to move this exact project."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	ctx = a.beginProjectResolution(ctx)
	current, err := a.resolver.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(source.LUID)})
	if err != nil {
		return Output{}, operationError("project.move.resolve", in, source.LUID, "Project revalidation failed.", "Review a new preview before moving.", err)
	}
	if current.Name != source.Name || current.Path != source.Path || current.ParentLUID != source.ParentLUID {
		return Output{}, operationError("project.move.target_changed", in, source.LUID, "The project changed during revalidation.", "Review a new preview before moving.", errors.New("project identity changed during revalidation"))
	}
	destination, parentLUID, err = a.destinationByLUID(ctx, in, current, destination)
	if err != nil {
		return Output{}, err
	}
	if err = a.validateMove(ctx, in, current, destination, parentLUID); err != nil {
		return Output{}, err
	}
	out.Plan.Source, out.Plan.Destination, out.Plan.NoOp = current, destination, current.ParentLUID == *parentLUID
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", Project: current}
		return out, nil
	}
	result, err := a.mover.MoveProject(ctx, current.LUID, parentLUID)
	if err != nil {
		return Output{}, operationError("project.move.failed", in, current.LUID, "Project move failed.", "Inspect the exact project before retrying: "+commandhint.Environment(in.Environment, "content", "project", "inspect", "--project-id", current.LUID), err)
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "content", "project", "inspect", "--project-id", current.LUID)}
	return out, nil
}
func (a *Action) destination(ctx context.Context, in Input, source Project) (*Project, *string, error) {
	if in.TopLevel {
		parent := ""
		return nil, &parent, nil
	}
	project, err := a.resolver.ResolveProject(ctx, in.ParentSelector)
	if err != nil {
		return nil, nil, operationError("project.move.parent", in, source.LUID, "Parent project resolution failed.", "Review the exact parent project, then retry.", err)
	}
	return &project, &project.LUID, nil
}
func (a *Action) destinationByLUID(ctx context.Context, in Input, source Project, destination *Project) (*Project, *string, error) {
	if in.TopLevel {
		return a.destination(ctx, in, source)
	}
	copy := in
	copy.ParentSelector = identity.Selector{LUID: identity.LUID(destination.LUID)}
	return a.destination(ctx, copy, source)
}
func (a *Action) validateMove(ctx context.Context, in Input, source Project, destination *Project, parentLUID *string) error {
	if parentLUID == nil {
		return usage("parent", "project move requires an exact parent or --top-level")
	}
	if *parentLUID == source.LUID {
		return operationError("project.move.cycle", in, source.LUID, "A project cannot be its own parent.", "Choose another parent project.", errors.New("project hierarchy cycle"))
	}
	if destination != nil && (destination.Path == source.Path || strings.HasPrefix(destination.Path, source.Path+"/")) {
		return operationError("project.move.cycle", in, source.LUID, "A project cannot move under its descendant.", "Choose a project outside the source hierarchy.", errors.New("project hierarchy cycle"))
	}
	if source.ParentLUID == *parentLUID {
		return nil
	}
	matches, err := a.resolver.FindProjectCollisions(ctx, source.Name, *parentLUID)
	if err != nil {
		return operationError("project.move.collision", in, source.LUID, "Project collision check failed.", "Review the destination hierarchy before moving.", err)
	}
	for _, match := range matches {
		if match.LUID != source.LUID {
			return operationError("project.move.collision", in, source.LUID, "The destination already contains this project name.", "Rename the project or choose another parent.", errors.New("exact sibling project name collision"))
		}
	}
	return nil
}
func validate(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return usage("environment", "project move requires an explicit resolved environment and site")
	}
	return ValidateInput(in)
}

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" {
		return usage("environment", "project move requires an explicit environment")
	}
	if in.ProjectSelector.LUID == "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) == "" {
		return usage("selector", "project move requires a project LUID or exact path")
	}
	if in.ProjectSelector.LUID != "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) != "" {
		return usage("selector", "a project LUID cannot be combined with a project path")
	}
	hasParent := in.ParentSelector.LUID != "" || strings.TrimSpace(in.ParentSelector.ProjectPath) != ""
	if hasParent == in.TopLevel {
		return usage("parent", "use exactly one parent project selector or --top-level")
	}
	if in.ParentSelector.LUID != "" && strings.TrimSpace(in.ParentSelector.ProjectPath) != "" {
		return usage("parent", "a parent project LUID cannot be combined with a project path")
	}
	return nil
}
func usage(field, message string) error {
	return &errs.Error{ID: "project.move.usage", Kind: errs.KindUsage, Operation: "project.move", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func operationError(id string, in Input, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "project.move", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
