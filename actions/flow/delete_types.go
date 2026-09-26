package flow

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type DeleteInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved    bool
	Environment, Site string
	Selector          identity.Selector
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *DeleteInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

type DeleteFlow = value.ContentIdentity
type DeletePlan struct {
	Mode        string     `json:"mode"`
	Operation   string     `json:"operation"`
	Environment string     `json:"environment"`
	Site        string     `json:"site"`
	Target      DeleteFlow `json:"target"`
}
type DeleteResult struct {
	Status           string `json:"status"`
	FlowLUID         string `json:"flow_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type DeleteOutput struct {
	Plan   DeletePlan    `json:"plan"`
	Result *DeleteResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type DeleteCompactDeleteResult struct {
	Status   string `json:"status"`
	FlowLUID string `json:"flow_luid"`
}
type DeleteCompactResult struct {
	Plan    DeletePlan                 `json:"plan"`
	Result  *DeleteCompactDeleteResult `json:"result,omitempty"`
	Details string                     `json:"details"`
	Help    []string                   `json:"help"`
}
type DeleteFullResult struct {
	Plan   DeletePlan    `json:"plan"`
	Result *DeleteResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

func (o DeleteOutput) CompactOutput() any {
	var result *DeleteCompactDeleteResult
	if o.Result != nil {
		result = &DeleteCompactDeleteResult{Status: o.Result.Status, FlowLUID: o.Result.FlowLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

func (o DeleteOutput) FullOutput() any {
	return DeleteFullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}
