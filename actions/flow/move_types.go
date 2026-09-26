package flow

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type MoveInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                bool
	Environment, Site             string
	FlowSelector, ProjectSelector identity.Selector
}

// SetFlowSelector records one exact source without exposing identity plumbing to Cobra.
func (i *MoveInput) SetFlowSelector(luid, name, projectPath string) {
	i.FlowSelector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// SetProjectSelector records one exact destination without exposing identity plumbing to Cobra.
func (i *MoveInput) SetProjectSelector(luid, projectPath string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

type MoveFlow = value.ContentIdentity
type MovePlan struct {
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	Source      MoveFlow `json:"source"`
	Destination Project  `json:"destination"`
	NoOp        bool     `json:"no_op"`
}
type MoveResult struct {
	Status           string `json:"status"`
	FlowLUID         string `json:"flow_luid"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type MoveOutput struct {
	Plan   MovePlan    `json:"plan"`
	Result *MoveResult `json:"result,omitempty"`
	Help   []string    `json:"help"`
}
type MoveCompactMoveResult struct {
	Status      string `json:"status"`
	FlowLUID    string `json:"flow_luid"`
	ProjectLUID string `json:"project_luid"`
}
type MoveCompactResult struct {
	Plan    MovePlan               `json:"plan"`
	Result  *MoveCompactMoveResult `json:"result,omitempty"`
	Details string                 `json:"details"`
	Help    []string               `json:"help"`
}
type MoveFullResult struct {
	Plan   MovePlan    `json:"plan"`
	Result *MoveResult `json:"result,omitempty"`
	Help   []string    `json:"help"`
}

func (o MoveOutput) CompactOutput() any {
	var result *MoveCompactMoveResult
	if o.Result != nil {
		result = &MoveCompactMoveResult{
			Status:      o.Result.Status,
			FlowLUID:    o.Result.FlowLUID,
			ProjectLUID: o.Result.ProjectLUID,
		}
	}
	return MoveCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

func (o MoveOutput) FullOutput() any {
	return MoveFullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}
