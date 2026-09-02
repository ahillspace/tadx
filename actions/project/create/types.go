package create

import "github.com/ahillspace/tadx/internal/identity"

// Input describes one project creation and its optional exact parent.
type Input struct {
	Environment        string
	Site               string
	Name               string
	Description        string
	ContentPermissions string
	ParentSelector     identity.Selector
}

// SetParentSelector records an optional exact parent selector.
func (i *Input) SetParentSelector(luid, projectPath string) {
	i.ParentSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

// Project is one authoritative project identity and bounded metadata projection.
type Project struct {
	LUID               string `json:"luid"`
	Name               string `json:"name"`
	Path               string `json:"path"`
	ParentLUID         string `json:"parent_luid,omitempty"`
	Description        string `json:"description,omitempty"`
	ContentPermissions string `json:"content_permissions,omitempty"`
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

// Plan is the complete bounded project-create preview.
type Plan struct {
	Mode        string      `json:"mode"`
	Operation   string      `json:"operation"`
	Environment string      `json:"environment"`
	Site        string      `json:"site"`
	Project     ProjectSpec `json:"project"`
	Parent      *Project    `json:"parent,omitempty"`
}

// Result is the authoritative project-create result.
type Result struct {
	Status           string  `json:"status"`
	Project          Project `json:"project"`
	TableauRequestID string  `json:"tableau_request_id,omitempty"`
}

// Output retains complete details before projection.
type Output struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}

// CompactProject is the exact identity needed by a later action.
type CompactProject struct {
	LUID       string `json:"luid"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	ParentLUID string `json:"parent_luid,omitempty"`
}

// CompactMutationResult omits successful request diagnostics.
type CompactMutationResult struct {
	Status  string         `json:"status"`
	Project CompactProject `json:"project"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Plan    Plan                   `json:"plan"`
	Applied bool                   `json:"applied"`
	Result  *CompactMutationResult `json:"result,omitempty"`
	Details string                 `json:"details"`
	Help    []string               `json:"help"`
}

// FullResult is the expanded projection.
type FullResult = Output

// CompactOutput returns exact mutation identity without the successful request ID.
func (o Output) CompactOutput() any {
	var result *CompactMutationResult
	if o.Result != nil {
		result = &CompactMutationResult{Status: o.Result.Status, Project: CompactProject{LUID: o.Result.Project.LUID, Name: o.Result.Project.Name, Path: o.Result.Project.Path, ParentLUID: o.Result.Project.ParentLUID}}
	}
	return CompactResult{Plan: o.Plan, Applied: o.Applied, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded mutation details.
func (o Output) FullOutput() any { return FullResult(o) }
