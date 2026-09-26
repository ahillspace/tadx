package flow

import "github.com/ahillspace/tadx/internal/identity"

type UpdateInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved    bool
	Environment, Site string
	Selector          identity.Selector
	OwnerLUID         *string
}

func (i *UpdateInput) SetSelector(luid, name, path string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: path}
}

type UpdateFlow struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	OwnerLUID   string `json:"owner_luid"`
}
type UpdateRequest struct {
	LUID      string
	OwnerLUID *string
}
type UpdateChange struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type UpdatePlan struct {
	Mode        string         `json:"mode"`
	Operation   string         `json:"operation"`
	Environment string         `json:"environment"`
	Site        string         `json:"site"`
	Target      UpdateFlow     `json:"target"`
	Changes     []UpdateChange `json:"changes"`
	NoOp        bool           `json:"no_op"`
}
type UpdateResult struct {
	Status           string `json:"status"`
	FlowLUID         string `json:"flow_luid"`
	FlowName         string `json:"flow_name"`
	ProjectLUID      string `json:"project_luid"`
	OwnerLUID        string `json:"owner_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type updateCompactResult struct {
	Status      string `json:"status"`
	FlowLUID    string `json:"flow_luid"`
	FlowName    string `json:"flow_name"`
	ProjectLUID string `json:"project_luid"`
	OwnerLUID   string `json:"owner_luid"`
}
type updateCompactOutput struct {
	Plan    UpdatePlan           `json:"plan"`
	Result  *updateCompactResult `json:"result,omitempty"`
	Details string               `json:"details"`
	Help    []string             `json:"help"`
}

func (o UpdateOutput) CompactOutput() any {
	var r *updateCompactResult
	if o.Result != nil {
		r = &updateCompactResult{Status: o.Result.Status, FlowLUID: o.Result.FlowLUID, FlowName: o.Result.FlowName, ProjectLUID: o.Result.ProjectLUID, OwnerLUID: o.Result.OwnerLUID}
	}
	return updateCompactOutput{Plan: o.Plan, Result: r, Details: "--full", Help: o.Help}
}
func (o UpdateOutput) FullOutput() any { return o }
