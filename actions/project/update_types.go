package project

import "github.com/ahillspace/tadx/internal/identity"

// Input selects one project and explicit bounded metadata changes.
type UpdateInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved     bool
	Environment        string
	Site               string
	Selector           identity.Selector
	Name               *string
	Description        *string
	ContentPermissions *string
}

// SetSelector records one exact project selector.
func (i *UpdateInput) SetSelector(luid, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

// UpdateProject shares the equivalent create/update identity observation.
type UpdateProject = MutationProject

// Changes contains only explicit requested metadata fields.
type UpdateChanges struct {
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	ContentPermissions *string `json:"content_permissions,omitempty"`
}

// UpdateRequest carries only changed fields to the released REST adapter.
type UpdateRequest struct {
	LUID               string
	Name               *string
	Description        *string
	ContentPermissions *string
}

// Plan is the complete bounded project-update preview.
type UpdatePlan struct {
	Mode        string        `json:"mode"`
	Operation   string        `json:"operation"`
	Environment string        `json:"environment"`
	Site        string        `json:"site"`
	Target      UpdateProject `json:"target"`
	Changes     UpdateChanges `json:"changes"`
	NoOp        bool          `json:"no_op"`
}

// Result is the authoritative project-update result.
type UpdateResult struct {
	Status           string        `json:"status"`
	Project          UpdateProject `json:"project"`
	TableauRequestID string        `json:"tableau_request_id,omitempty"`
}

// Output retains complete details before projection.
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

// CompactProject is the exact identity needed by a later action.
type UpdateCompactProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// CompactMutationResult omits successful request diagnostics.
type UpdateCompactMutationResult struct {
	Status  string               `json:"status"`
	Project UpdateCompactProject `json:"project"`
}

// CompactResult is the default projection.
type UpdateCompactResult struct {
	Plan    UpdatePlan                   `json:"plan"`
	Result  *UpdateCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}

// FullResult is the expanded projection.
type UpdateFullResult = UpdateOutput

// CompactOutput returns exact mutation identity without the successful request ID.
func (o UpdateOutput) CompactOutput() any {
	var result *UpdateCompactMutationResult
	if o.Result != nil {
		result = &UpdateCompactMutationResult{Status: o.Result.Status, Project: UpdateCompactProject{LUID: o.Result.Project.LUID, Name: o.Result.Project.Name, Path: o.Result.Project.Path, ParentLUID: o.Result.Project.ParentLUID, ContentPermissions: o.Result.Project.ContentPermissions, ControllingPermissionsProjectID: o.Result.Project.ControllingPermissionsProjectID}}
	}
	return UpdateCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded mutation details.
func (o UpdateOutput) FullOutput() any { return UpdateFullResult(o) }
