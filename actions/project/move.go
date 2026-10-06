package project

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type MoveResolver interface {
	ResolveProject(context.Context, identity.Selector) (MoveProject, error)
	FindProjectCollisions(context.Context, string, string) ([]MoveProject, error)
}
type Mover interface {
	MoveProject(context.Context, string, *string) (MoveResult, error)
}
type MoveAction struct {
	resolver MoveResolver
	mover    Mover
}

func NewMove(r MoveResolver, m Mover) *MoveAction { return &MoveAction{r, m} }

// A resolver may share project reads within one explicit validation phase.
// Each prewrite phase starts again, never inheriting the planning snapshot.
type projectResolutionPhase interface {
	BeginProjectResolution(context.Context) context.Context
}

func (a *MoveAction) beginProjectResolution(ctx context.Context) context.Context {
	if a == nil {
		return ctx
	}
	if resolver, ok := a.resolver.(projectResolutionPhase); ok {
		return resolver.BeginProjectResolution(ctx)
	}
	return ctx
}

func (a *MoveAction) Execute(ctx context.Context, in MoveInput, preview bool) (MoveOutput, error) {
	ctx = a.beginProjectResolution(ctx)
	if a == nil || a.resolver == nil || a.mover == nil {
		return MoveOutput{}, &errs.Error{ID: "project.move.unconfigured", Kind: errs.KindRuntime, Operation: "project.move", Summary: "Project move is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project move before retrying."}
	}
	if err := validate(in); err != nil {
		return MoveOutput{}, err
	}
	source, err := a.resolver.ResolveProject(ctx, in.ProjectSelector)
	if err != nil {
		return MoveOutput{}, moveOperationError("project.move.resolve", in, "", "Project resolution failed.", "Review the exact project selector, then retry.", err)
	}
	destination, parentLUID, err := a.destination(ctx, in, source)
	if err != nil {
		return MoveOutput{}, err
	}
	if err = a.validateMove(ctx, in, source, destination, parentLUID); err != nil {
		return MoveOutput{}, err
	}
	out := MoveOutput{Plan: MovePlan{Mode: "preview", Operation: "project.move", Environment: in.Environment, Site: in.Site, Source: source, Destination: destination, TopLevel: in.TopLevel, NoOp: source.ParentLUID == *parentLUID}, Help: []string{"Run without --preview to move this exact project."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	ctx = a.beginProjectResolution(ctx)
	current, err := a.resolver.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(source.LUID)})
	if err != nil {
		return MoveOutput{}, moveOperationError("project.move.resolve", in, source.LUID, "Project revalidation failed.", "Review a new preview before moving.", err)
	}
	if current.Name != source.Name || current.Path != source.Path || current.ParentLUID != source.ParentLUID {
		return MoveOutput{}, moveOperationError("project.move.target_changed", in, source.LUID, "The project changed during revalidation.", "Review a new preview before moving.", errors.New("project identity changed during revalidation"))
	}
	destination, parentLUID, err = a.destinationByLUID(ctx, in, current, destination)
	if err != nil {
		return MoveOutput{}, err
	}
	if err = a.validateMove(ctx, in, current, destination, parentLUID); err != nil {
		return MoveOutput{}, err
	}
	out.Plan.Source, out.Plan.Destination, out.Plan.NoOp = current, destination, current.ParentLUID == *parentLUID
	if out.Plan.NoOp {
		out.Result = &MoveResult{Status: "unchanged", Project: current}
		return out, nil
	}
	result, err := a.mover.MoveProject(ctx, current.LUID, parentLUID)
	if err != nil {
		return MoveOutput{}, moveOperationError("project.move.failed", in, current.LUID, "Project move failed.", "Inspect the exact project before retrying: "+commandhint.Environment(in.Environment, "content", "project", "inspect", "--project-id", current.LUID), err)
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "content", "project", "inspect", "--project-id", current.LUID)}
	return out, nil
}
func (a *MoveAction) destination(ctx context.Context, in MoveInput, source MoveProject) (*MoveProject, *string, error) {
	if in.TopLevel {
		parent := ""
		return nil, &parent, nil
	}
	project, err := a.resolver.ResolveProject(ctx, in.ParentSelector)
	if err != nil {
		return nil, nil, moveOperationError("project.move.parent", in, source.LUID, "Parent project resolution failed.", "Review the exact parent project, then retry.", err)
	}
	return &project, &project.LUID, nil
}
func (a *MoveAction) destinationByLUID(ctx context.Context, in MoveInput, source MoveProject, destination *MoveProject) (*MoveProject, *string, error) {
	if in.TopLevel {
		return a.destination(ctx, in, source)
	}
	copy := in
	copy.ParentSelector = identity.Selector{LUID: identity.LUID(destination.LUID)}
	return a.destination(ctx, copy, source)
}
func (a *MoveAction) validateMove(ctx context.Context, in MoveInput, source MoveProject, destination *MoveProject, parentLUID *string) error {
	if parentLUID == nil {
		return moveUsage("parent", "project move requires an exact parent or --top-level")
	}
	if *parentLUID == source.LUID {
		return moveOperationError("project.move.cycle", in, source.LUID, "A project cannot be its own parent.", "Choose another parent project.", errors.New("project hierarchy cycle"))
	}
	if destination != nil && (destination.Path == source.Path || strings.HasPrefix(destination.Path, source.Path+"/")) {
		return moveOperationError("project.move.cycle", in, source.LUID, "A project cannot move under its descendant.", "Choose a project outside the source hierarchy.", errors.New("project hierarchy cycle"))
	}
	if source.ParentLUID == *parentLUID {
		return nil
	}
	matches, err := a.resolver.FindProjectCollisions(ctx, source.Name, *parentLUID)
	if err != nil {
		return moveOperationError("project.move.collision", in, source.LUID, "Project collision check failed.", "Review the destination hierarchy before moving.", err)
	}
	for _, match := range matches {
		if match.LUID != source.LUID {
			return moveOperationError("project.move.collision", in, source.LUID, "The destination already contains this project name.", "Rename the project or choose another parent.", errors.New("exact sibling project name collision"))
		}
	}
	return nil
}
func validate(in MoveInput) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return moveUsage("environment", "project move requires an explicit resolved environment and site")
	}
	return ValidateMoveInput(in)
}

// ValidateMoveInput checks caller-controlled arguments before local or remote setup.
func ValidateMoveInput(in MoveInput) error {
	if strings.TrimSpace(in.Environment) == "" {
		return moveUsage("environment", "project move requires an explicit environment")
	}
	if in.ProjectSelector.LUID == "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) == "" {
		return moveUsage("selector", "project move requires a project LUID or exact path")
	}
	if in.ProjectSelector.LUID != "" && strings.TrimSpace(in.ProjectSelector.ProjectPath) != "" {
		return moveUsage("selector", "a project LUID cannot be combined with a project path")
	}
	hasParent := in.ParentSelector.LUID != "" || strings.TrimSpace(in.ParentSelector.ProjectPath) != ""
	if hasParent == in.TopLevel {
		return moveUsage("parent", "use exactly one parent project selector or --top-level")
	}
	if in.ParentSelector.LUID != "" && strings.TrimSpace(in.ParentSelector.ProjectPath) != "" {
		return moveUsage("parent", "a parent project LUID cannot be combined with a project path")
	}
	return nil
}
func moveUsage(field, message string) error {
	return &errs.Error{ID: "project.move.usage", Kind: errs.KindUsage, Operation: "project.move", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func moveOperationError(id string, in MoveInput, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "project.move", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

type MoveInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                  bool
	Environment, Site               string
	ProjectSelector, ParentSelector identity.Selector
	TopLevel                        bool
}

func (i *MoveInput) SetProjectSelector(luid, path string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: path}
}
func (i *MoveInput) SetParentSelector(luid, path string) {
	i.ParentSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: path}
}

type MoveProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	PathUnavailableReason           string `json:"path_unavailable_reason,omitempty"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}
type MovePlan struct {
	Mode        string       `json:"mode"`
	Operation   string       `json:"operation"`
	Environment string       `json:"environment"`
	Site        string       `json:"site"`
	Source      MoveProject  `json:"source"`
	Destination *MoveProject `json:"destination,omitempty"`
	TopLevel    bool         `json:"top_level"`
	NoOp        bool         `json:"no_op"`
}
type MoveResult struct {
	Status           string      `json:"status"`
	Project          MoveProject `json:"project"`
	TableauRequestID string      `json:"tableau_request_id,omitempty"`
}
type MoveOutput struct {
	Plan   MovePlan    `json:"plan"`
	Result *MoveResult `json:"result,omitempty"`
	Help   []string    `json:"help"`
}
type compactResult struct {
	Status  string      `json:"status"`
	Project MoveProject `json:"project"`
}
type compactOutput struct {
	Plan    MovePlan       `json:"plan"`
	Result  *compactResult `json:"result,omitempty"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

func (o MoveOutput) CompactOutput() any {
	var r *compactResult
	if o.Result != nil {
		r = &compactResult{Status: o.Result.Status, Project: o.Result.Project}
	}
	return compactOutput{Plan: o.Plan, Result: r, Details: "--full", Help: o.Help}
}
func (o MoveOutput) FullOutput() any { return o }
