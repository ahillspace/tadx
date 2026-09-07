package delete

import "github.com/ahillspace/tadx/internal/identity"

// Input selects one exact workbook on one explicit Tableau target.
type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved bool
	Environment    string
	Site           string
	Selector       identity.Selector
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// Workbook is the authoritative delete target shown during preview.
type Workbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
}

// Plan is the immutable preview of one workbook deletion.
type Plan struct {
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	Target      Workbook `json:"target"`
}

// Result is the authoritative remote outcome.
type Result struct {
	Status           string `json:"status"`
	WorkbookLUID     string `json:"workbook_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

// Output contains preview and optional applied outcome.
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

// CompactDeleteResult omits successful request diagnostics.
type CompactDeleteResult struct {
	Status       string `json:"status"`
	WorkbookLUID string `json:"workbook_luid"`
}

// CompactResult is the bounded default projection.
type CompactResult struct {
	Plan    Plan                 `json:"plan"`
	Result  *CompactDeleteResult `json:"result,omitempty"`
	Details string               `json:"details"`
	Help    []string             `json:"help"`
}

// FullResult includes bounded upstream diagnostics.
type FullResult = Output

// CompactOutput returns the bounded default projection.
func (o Output) CompactOutput() any {
	var result *CompactDeleteResult
	if o.Result != nil {
		result = &CompactDeleteResult{Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID}
	}
	return CompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded request diagnostics.
func (o Output) FullOutput() any { return FullResult(o) }
