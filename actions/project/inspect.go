package project

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

// InspectResolver is the action-owned exact project seam.
type InspectResolver interface {
	ResolveProject(context.Context, identity.Selector) (InspectProject, error)
}

// InspectAction inspects one exact project.
type InspectAction struct{ resolver InspectResolver }

// NewInspect creates a project inspect action.
func NewInspect(resolver InspectResolver) *InspectAction { return &InspectAction{resolver: resolver} }

// ValidateInspectInput checks an exact selector before authentication.
func ValidateInspectInput(input InspectInput) error {
	if input.Selector.LUID == "" && input.Selector.ProjectPath == "" {
		return &errs.Error{ID: "project.inspect.usage", Kind: errs.KindUsage, Operation: "project.inspect", Summary: "project LUID or exact project path is required", Retryable: errs.Bool(false), CorrectiveAction: "Provide a project LUID or an exact project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "project LUID or exact project path is required"}}}
	}
	return nil
}

// Execute resolves one authoritative project.
func (a *InspectAction) Execute(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if a == nil || a.resolver == nil {
		return InspectOutput{}, &errs.Error{ID: "project.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "project.inspect", Summary: "Project inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the project resolver before retrying."}
	}
	if err := ValidateInspectInput(input); err != nil {
		return InspectOutput{}, err
	}
	project, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return InspectOutput{}, err
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
		return InspectOutput{}, &errs.Error{ID: "project.inspect.resolve", Kind: errs.KindOperation, Operation: "project.inspect", Environment: input.Environment, Site: input.Site, Summary: "Project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, Project: project, RequestID: project.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", project.LUID, "--full")}}, nil
}

// InspectInput selects one exact project.
type InspectInput struct {
	Environment string
	Site        string
	Selector    identity.Selector
	Cache       bool
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *InspectInput) SetSelector(luid, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

// InspectProject is one complete project projection.
type InspectProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid"`
	Description                     string `json:"description"`
	OwnerLUID                       string `json:"owner_luid,omitempty"`
	TopLevel                        *bool  `json:"top_level,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
	CreatedAt                       string `json:"created_at,omitempty"`
	UpdatedAt                       string `json:"updated_at,omitempty"`
	ProjectCount                    *int   `json:"project_count,omitempty"`
	WorkbookCount                   *int   `json:"workbook_count,omitempty"`
	ViewCount                       *int   `json:"view_count,omitempty"`
	DatasourceCount                 *int   `json:"datasource_count,omitempty"`
	RequestID                       string `json:"-"`
}

// InspectOutput retains complete details before projection.
type InspectOutput struct {
	Status      string
	Environment string
	Site        string
	Project     InspectProject
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// InspectCompactProject is the exact identity needed for another action.
type InspectCompactProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// InspectCompactResult is the default projection.
type InspectCompactResult struct {
	Status      string                `json:"status"`
	Environment string                `json:"environment,omitempty"`
	Site        string                `json:"site,omitempty"`
	Project     InspectCompactProject `json:"project"`
	Details     string                `json:"details"`
	Help        []string              `json:"help"`
	Source      *readsource.Metadata  `json:"source,omitempty"`
}

// InspectFullResult is the expanded projection.
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Project     InspectProject       `json:"project"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns exact identity fields.
func (o InspectOutput) CompactOutput() any {
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Project: InspectCompactProject{LUID: o.Project.LUID, Name: o.Project.Name, Path: o.Project.Path, ParentLUID: o.Project.ParentLUID, ContentPermissions: o.Project.ContentPermissions, ControllingPermissionsProjectID: o.Project.ControllingPermissionsProjectID}, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns bounded lifecycle details.
func (o InspectOutput) FullOutput() any {
	project := o.Project
	if project.TopLevel == nil && project.ParentLUID == "" {
		project.TopLevel = new(true)
	}
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Project: project, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
