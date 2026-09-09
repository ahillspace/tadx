package delete

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved    bool
	Environment, Site string
	Selector          identity.Selector
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

type Flow = value.ContentIdentity
type Plan struct {
	Mode        string `json:"mode"`
	Operation   string `json:"operation"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	Target      Flow   `json:"target"`
}
type Result struct {
	Status           string `json:"status"`
	FlowLUID         string `json:"flow_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type CompactDeleteResult struct {
	Status   string `json:"status"`
	FlowLUID string `json:"flow_luid"`
}
type CompactResult struct {
	Plan    Plan                 `json:"plan"`
	Result  *CompactDeleteResult `json:"result,omitempty"`
	Details string               `json:"details"`
	Help    []string             `json:"help"`
}
type FullResult struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactDeleteResult
	if o.Result != nil {
		result = &CompactDeleteResult{Status: o.Result.Status, FlowLUID: o.Result.FlowLUID}
	}
	return CompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}
