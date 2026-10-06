package project

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// CreateResolver owns exact parent resolution and sibling collision checks.
type CreateResolver interface {
	ResolveProject(context.Context, identity.Selector) (CreateProject, error)
	FindProjectCollisions(context.Context, string, string) ([]CreateProject, error)
}

// Creator performs one exact released project create request.
type Creator interface {
	CreateProject(context.Context, CreateRequest) (CreateResult, error)
}

// CreateAction creates one project after immediate revalidation.
type CreateAction struct {
	resolver CreateResolver
	creator  Creator
}

// NewCreate creates a project-create action.
func NewCreate(resolver CreateResolver, creator Creator) *CreateAction {
	return &CreateAction{resolver: resolver, creator: creator}
}

// A resolver may share project reads within one explicit validation phase.
// Each prewrite phase starts again, never inheriting the planning snapshot.
type createProjectResolutionPhase interface {
	BeginProjectResolution(context.Context) context.Context
}

func (a *CreateAction) beginProjectResolution(ctx context.Context) context.Context {
	if a == nil {
		return ctx
	}
	if resolver, ok := a.resolver.(createProjectResolutionPhase); ok {
		return resolver.BeginProjectResolution(ctx)
	}
	return ctx
}

// Execute previews or creates one exact project.
func (a *CreateAction) Execute(ctx context.Context, input CreateInput, preview bool) (CreateOutput, error) {
	ctx = a.beginProjectResolution(ctx)
	if a == nil || a.resolver == nil || a.creator == nil {
		return CreateOutput{}, &errs.Error{ID: "project.create.unconfigured", Kind: errs.KindRuntime, Operation: "project.create", Summary: "Project create is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project creation before retrying."}
	}
	if err := validateCreateInput(input); err != nil {
		return CreateOutput{}, err
	}
	parent, err := a.resolveParent(ctx, input.ParentSelector)
	if err != nil {
		return CreateOutput{}, createResolutionError(input, "Parent project resolution failed.", err)
	}
	parentLUID := ""
	if parent != nil {
		parentLUID = parent.LUID
	}
	if err := a.rejectCollision(ctx, input, parentLUID); err != nil {
		return CreateOutput{}, err
	}
	plan := CreatePlan{Mode: "preview", Operation: "project.create", Environment: input.Environment, Site: input.Site, Project: ProjectSpec{Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions}, Parent: parent}
	output := CreateOutput{Plan: plan, Help: []string{"Run without --preview to create this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	ctx = a.beginProjectResolution(ctx)
	currentParent, err := a.resolveParent(ctx, input.ParentSelector)
	if err != nil {
		return CreateOutput{}, createResolutionError(input, "Parent project revalidation failed.", err)
	}
	currentParentLUID := ""
	if currentParent != nil {
		currentParentLUID = currentParent.LUID
	}
	if currentParentLUID != parentLUID {
		return CreateOutput{}, &errs.Error{ID: "project.create.parent_changed", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: "The project parent identity changed during revalidation.", Cause: errors.New("project parent LUID changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before creating the project."}
	}
	if err := a.rejectCollision(ctx, input, currentParentLUID); err != nil {
		return CreateOutput{}, err
	}
	result, err := a.creator.CreateProject(ctx, CreateRequest{Name: input.Name, Description: input.Description, ParentLUID: currentParentLUID, ContentPermissions: input.ContentPermissions})
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the remote project create outcome before retrying.")
		return CreateOutput{}, &errs.Error{ID: "project.create.failed", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: "Project create failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Plan.Parent = currentParent
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", result.Project.LUID)}
	return output, nil
}

func (a *CreateAction) resolveParent(ctx context.Context, selector identity.Selector) (*CreateProject, error) {
	if selector.LUID == "" && strings.TrimSpace(selector.ProjectPath) == "" {
		return nil, nil
	}
	project, err := a.resolver.ResolveProject(ctx, selector)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(project.LUID) == "" {
		return nil, errors.New("parent project resolution omitted the authoritative LUID")
	}
	return &project, nil
}

func (a *CreateAction) rejectCollision(ctx context.Context, input CreateInput, parentLUID string) error {
	matches, err := a.resolver.FindProjectCollisions(ctx, input.Name, parentLUID)
	if err != nil {
		return createResolutionError(input, "Project collision check failed.", err)
	}
	if len(matches) == 0 {
		return nil
	}
	return &errs.Error{ID: "project.create.collision", Kind: errs.KindOperation, Operation: "project.create", Resource: matches[0].LUID, Environment: input.Environment, Site: input.Site, Summary: "A sibling project with the same case-insensitive name already exists.", Cause: fmt.Errorf("project %q already exists under the selected parent", matches[0].LUID), Retryable: errs.Bool(false), CorrectiveAction: "Choose a different name or exact parent, then review a new preview."}
}

func validateCreateInput(input CreateInput) error {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return createUsage("environment", "project create requires an explicit resolved environment and site")
	}
	return ValidateCreateInput(input)
}

// ValidateCreateInput checks caller-controlled arguments before local or remote setup.
func ValidateCreateInput(input CreateInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return createUsage("environment", "project create requires an explicit environment")
	}
	if strings.TrimSpace(input.Name) == "" {
		return createUsage("name", "project create requires a name")
	}
	if strings.Contains(input.Name, "/") {
		return createUsage("name", "project create name cannot contain a slash")
	}
	if input.ParentSelector.Name != "" || (input.ParentSelector.LUID != "" && strings.TrimSpace(input.ParentSelector.ProjectPath) != "") {
		return createUsage("parent", "use either a parent LUID or an exact parent project path")
	}
	if !validCreateContentPermissions(input.ContentPermissions) {
		return createUsage("content_permissions", "project create content permissions are invalid")
	}
	return nil
}

func validCreateContentPermissions(value string) bool {
	return value == "" || value == "ManagedByOwner" || value == "LockedToProject" || value == "LockedToProjectWithoutNested"
}

func createResolutionError(input CreateInput, summary string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
	return &errs.Error{ID: "project.create.resolve", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

func createUsage(field, message string) error {
	return &errs.Error{ID: "project.create.usage", Kind: errs.KindUsage, Operation: "project.create", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project create input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}

// CreateInput describes one project creation and its optional exact parent.
type CreateInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved     bool
	Environment        string
	Site               string
	Name               string
	Description        string
	ContentPermissions string
	ParentSelector     identity.Selector
}

// SetParentSelector records an optional exact parent selector.
func (i *CreateInput) SetParentSelector(luid, projectPath string) {
	i.ParentSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

// CreateProject is one authoritative project identity and bounded metadata projection.
type CreateProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	Description                     string `json:"description,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// CreateRequest carries the exact admitted V1 mutation fields.
type CreateRequest struct {
	Name               string
	Description        string
	ParentLUID         string
	ContentPermissions string
}

// ProjectSpec is the requested project metadata shown in preview.
type ProjectSpec struct {
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	ContentPermissions string `json:"content_permissions,omitempty"`
}

// CreatePlan is the complete bounded project-create preview.
type CreatePlan struct {
	Mode        string         `json:"mode"`
	Operation   string         `json:"operation"`
	Environment string         `json:"environment"`
	Site        string         `json:"site"`
	Project     ProjectSpec    `json:"project"`
	Parent      *CreateProject `json:"parent,omitempty"`
}

// CreateResult is the authoritative project-create result.
type CreateResult struct {
	Status           string        `json:"status"`
	Project          CreateProject `json:"project"`
	TableauRequestID string        `json:"tableau_request_id,omitempty"`
}

// CreateOutput retains complete details before projection.
type CreateOutput struct {
	Plan   CreatePlan    `json:"plan"`
	Result *CreateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

// CreateCompactProject is the exact identity needed by a later action.
type CreateCompactProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// CreateCompactMutationResult omits successful request diagnostics.
type CreateCompactMutationResult struct {
	Status  string               `json:"status"`
	Project CreateCompactProject `json:"project"`
}

// CreateCompactResult is the default projection.
type CreateCompactResult struct {
	Plan    CreatePlan                   `json:"plan"`
	Result  *CreateCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}

// CreateFullResult is the expanded projection.
type CreateFullResult = CreateOutput

// CompactOutput returns exact mutation identity without the successful request ID.
func (o CreateOutput) CompactOutput() any {
	var result *CreateCompactMutationResult
	if o.Result != nil {
		result = &CreateCompactMutationResult{Status: o.Result.Status, Project: CreateCompactProject{LUID: o.Result.Project.LUID, Name: o.Result.Project.Name, Path: o.Result.Project.Path, ParentLUID: o.Result.Project.ParentLUID, ContentPermissions: o.Result.Project.ContentPermissions, ControllingPermissionsProjectID: o.Result.Project.ControllingPermissionsProjectID}}
	}
	return CreateCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded mutation details.
func (o CreateOutput) FullOutput() any { return CreateFullResult(o) }
