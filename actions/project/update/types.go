package update

import "github.com/ahillspace/tadx/internal/identity"

// Input selects one project and explicit bounded metadata changes.
type Input struct {
	Environment        string
	Site               string
	Selector           identity.Selector
	Name               *string
	Description        *string
	ContentPermissions *string
}

// SetSelector records one exact project selector.
func (i *Input) SetSelector(luid, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
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

// Changes contains only explicit requested metadata fields.
type Changes struct {
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
type Plan struct {
	Mode        string  `json:"mode"`
	Operation   string  `json:"operation"`
	Environment string  `json:"environment"`
	Site        string  `json:"site"`
	Target      Project `json:"target"`
	Changes     Changes `json:"changes"`
	NoOp        bool    `json:"no_op"`
}

// Result is the authoritative project-update result.
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
