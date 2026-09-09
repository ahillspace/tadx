package move

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                bool
	Environment, Site             string
	FlowSelector, ProjectSelector identity.Selector
}

// SetFlowSelector records one exact source without exposing identity plumbing to Cobra.
func (i *Input) SetFlowSelector(luid, name, projectPath string) {
	i.FlowSelector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// SetProjectSelector records one exact destination without exposing identity plumbing to Cobra.
func (i *Input) SetProjectSelector(luid, projectPath string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

type Flow = value.ContentIdentity
type Project = value.ProjectIdentity
type Plan struct {
	Mode        string  `json:"mode"`
	Operation   string  `json:"operation"`
	Environment string  `json:"environment"`
	Site        string  `json:"site"`
	Source      Flow    `json:"source"`
	Destination Project `json:"destination"`
	NoOp        bool    `json:"no_op"`
}
type Result struct {
	Status           string `json:"status"`
	FlowLUID         string `json:"flow_luid"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type CompactMoveResult struct {
	Status      string `json:"status"`
	FlowLUID    string `json:"flow_luid"`
	ProjectLUID string `json:"project_luid"`
}
type CompactResult struct {
	Plan    Plan               `json:"plan"`
	Result  *CompactMoveResult `json:"result,omitempty"`
	Details string             `json:"details"`
	Help    []string           `json:"help"`
}
type FullResult struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactMoveResult
	if o.Result != nil {
		result = &CompactMoveResult{
			Status:      o.Result.Status,
			FlowLUID:    o.Result.FlowLUID,
			ProjectLUID: o.Result.ProjectLUID,
		}
	}
	return CompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}
