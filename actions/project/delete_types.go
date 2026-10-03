// Package project owns project lifecycle operations.
package project

// CascadeWarning describes Tableau's documented project-deletion behavior.
const DeleteCascadeWarning = "Deleting this project also deletes all Tableau assets inside it. TADX cannot verify that the project is empty."

// Input selects one exact project on one resolved Tableau target.
type DeleteInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved bool
	Environment    string
	Site           string
	ProjectLUID    string
}

// Project is the authoritative delete target shown during preview.
type DeleteProject struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Plan is the immutable preview of one project deletion.
type DeletePlan struct {
	Mode        string        `json:"mode"`
	Operation   string        `json:"operation"`
	Environment string        `json:"environment"`
	Site        string        `json:"site"`
	Target      DeleteProject `json:"target"`
}

// Result is the authoritative remote outcome.
type DeleteResult struct {
	Status           string `json:"status"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

// Output contains the preview and optional applied outcome.
type DeleteOutput struct {
	Plan     DeletePlan    `json:"plan"`
	Result   *DeleteResult `json:"result,omitempty"`
	Warnings []string      `json:"warnings"`
	Help     []string      `json:"help"`
}

// CompactDeleteResult omits successful request diagnostics.
type DeleteCompactDeleteResult struct {
	Status      string `json:"status"`
	ProjectLUID string `json:"project_luid"`
}

// CompactResult is the bounded default projection.
type DeleteCompactResult struct {
	Plan     DeletePlan                 `json:"plan"`
	Result   *DeleteCompactDeleteResult `json:"result,omitempty"`
	Warnings []string                   `json:"warnings"`
	Details  string                     `json:"details"`
	Help     []string                   `json:"help"`
}

// CompactOutput returns the bounded default projection.
func (o DeleteOutput) CompactOutput() any {
	var result *DeleteCompactDeleteResult
	if o.Result != nil {
		result = &DeleteCompactDeleteResult{Status: o.Result.Status, ProjectLUID: o.Result.ProjectLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Warnings: append([]string(nil), o.Warnings...), Details: "--full", Help: o.Help}
}

// FullOutput returns bounded request diagnostics.
func (o DeleteOutput) FullOutput() any { return o }
