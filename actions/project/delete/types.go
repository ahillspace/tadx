// Package delete implements project.delete.
package delete

// CascadeWarning describes Tableau's documented project-deletion behavior.
const CascadeWarning = "Deleting this project also deletes all Tableau assets inside it. TADX cannot verify that the project is empty."

// Input selects one exact project on one resolved Tableau target.
type Input struct {
	Environment string
	Site        string
	ProjectLUID string
}

// Project is the authoritative delete target shown during preview.
type Project struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Plan is the immutable preview of one project deletion.
type Plan struct {
	Mode        string  `json:"mode"`
	Operation   string  `json:"operation"`
	Environment string  `json:"environment"`
	Site        string  `json:"site"`
	Target      Project `json:"target"`
}

// Result is the authoritative remote outcome.
type Result struct {
	Status           string `json:"status"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

// Output contains the preview and optional applied outcome.
type Output struct {
	Plan     Plan     `json:"plan"`
	Result   *Result  `json:"result,omitempty"`
	Warnings []string `json:"warnings"`
	Help     []string `json:"help"`
}

// CompactDeleteResult omits successful request diagnostics.
type CompactDeleteResult struct {
	Status      string `json:"status"`
	ProjectLUID string `json:"project_luid"`
}

// CompactResult is the bounded default projection.
type CompactResult struct {
	Plan     Plan                 `json:"plan"`
	Result   *CompactDeleteResult `json:"result,omitempty"`
	Warnings []string             `json:"warnings"`
	Details  string               `json:"details"`
	Help     []string             `json:"help"`
}

// CompactOutput returns the bounded default projection.
func (o Output) CompactOutput() any {
	var result *CompactDeleteResult
	if o.Result != nil {
		result = &CompactDeleteResult{Status: o.Result.Status, ProjectLUID: o.Result.ProjectLUID}
	}
	return CompactResult{Plan: o.Plan, Result: result, Warnings: append([]string(nil), o.Warnings...), Details: "--full", Help: o.Help}
}

// FullOutput returns bounded request diagnostics.
func (o Output) FullOutput() any { return o }
