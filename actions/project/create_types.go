package project

import "github.com/ahillspace/tadx/internal/identity"

// Input describes one project creation and its optional exact parent.
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

// MutationProject is the common identity and metadata observation for create and update.
type MutationProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	Description                     string `json:"description,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// CreateProject is the create operation's project observation.
type CreateProject = MutationProject

// CreateRequest carries the exact admitted V1 mutation fields.
type CreateRequest struct {
	Name               string
	Description        string
	ParentLUID         string
	ContentPermissions string
}

// ProjectSpec is the requested project metadata shown in preview.
type CreateProjectSpec struct {
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	ContentPermissions string `json:"content_permissions,omitempty"`
}

// Plan is the complete bounded project-create preview.
type CreatePlan struct {
	Mode        string            `json:"mode"`
	Operation   string            `json:"operation"`
	Environment string            `json:"environment"`
	Site        string            `json:"site"`
	Project     CreateProjectSpec `json:"project"`
	Parent      *CreateProject    `json:"parent,omitempty"`
}

// Result is the authoritative project-create result.
type CreateResult struct {
	Status           string        `json:"status"`
	Project          CreateProject `json:"project"`
	TableauRequestID string        `json:"tableau_request_id,omitempty"`
}

// Output retains complete details before projection.
type CreateOutput struct {
	Plan   CreatePlan    `json:"plan"`
	Result *CreateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

// CompactProject is the exact identity needed by a later action.
type CreateCompactProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// CompactMutationResult omits successful request diagnostics.
type CreateCompactMutationResult struct {
	Status  string               `json:"status"`
	Project CreateCompactProject `json:"project"`
}

// CompactResult is the default projection.
type CreateCompactResult struct {
	Plan    CreatePlan                   `json:"plan"`
	Result  *CreateCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}

// FullResult is the expanded projection.
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
