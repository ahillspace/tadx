package workbook

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// DeleteInput selects one exact workbook on one explicit Tableau target.
type DeleteInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved bool
	Environment    string
	Site           string
	Selector       identity.Selector
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *DeleteInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// DeletePlan is the immutable preview of one workbook deletion.
type DeletePlan struct {
	Mode        string         `json:"mode"`
	Operation   string         `json:"operation"`
	Environment string         `json:"environment"`
	Site        string         `json:"site"`
	Target      deleteWorkbook `json:"target"`
}

// DeleteResult is the authoritative remote outcome.
type DeleteResult struct {
	Status           string `json:"status"`
	WorkbookLUID     string `json:"workbook_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

// DeleteOutput contains preview and optional applied outcome.
type DeleteOutput struct {
	Plan   DeletePlan    `json:"plan"`
	Result *DeleteResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

// DeleteCompactDeleteResult omits successful request diagnostics.
type DeleteCompactDeleteResult struct {
	Status       string `json:"status"`
	WorkbookLUID string `json:"workbook_luid"`
}

// DeleteCompactResult is the bounded default projection.
type DeleteCompactResult struct {
	Plan    DeletePlan                 `json:"plan"`
	Result  *DeleteCompactDeleteResult `json:"result,omitempty"`
	Details string                     `json:"details"`
	Help    []string                   `json:"help"`
}

// DeleteFullResult includes bounded upstream diagnostics.
type DeleteFullResult = DeleteOutput

// CompactOutput returns the bounded default projection.
func (o DeleteOutput) CompactOutput() any {
	var result *DeleteCompactDeleteResult
	if o.Result != nil {
		result = &DeleteCompactDeleteResult{Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded request diagnostics.
func (o DeleteOutput) FullOutput() any { return DeleteFullResult(o) }

type deleteWorkbook = value.ContentIdentity
