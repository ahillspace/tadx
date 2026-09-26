package datasource

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type UpdateInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved    bool
	Environment, Site string
	Selector          identity.Selector
	Name, OwnerLUID   *string
}

func (i *UpdateInput) SetSelector(luid, name, path string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: path}
}

type UpdateRequest struct {
	LUID            string
	Name, OwnerLUID *string
}
type UpdateChange struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type UpdatePlan struct {
	Mode        string           `json:"mode"`
	Operation   string           `json:"operation"`
	Environment string           `json:"environment"`
	Site        string           `json:"site"`
	Target      updateDatasource `json:"target"`
	Changes     []UpdateChange   `json:"changes"`
	NoOp        bool             `json:"no_op"`
}
type UpdateResult struct {
	Status           string `json:"status"`
	DatasourceLUID   string `json:"datasource_luid"`
	DatasourceName   string `json:"datasource_name"`
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
	Status         string `json:"status"`
	DatasourceLUID string `json:"datasource_luid"`
	DatasourceName string `json:"datasource_name"`
	ProjectLUID    string `json:"project_luid"`
	OwnerLUID      string `json:"owner_luid"`
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
		r = &updateCompactResult{Status: o.Result.Status, DatasourceLUID: o.Result.DatasourceLUID, DatasourceName: o.Result.DatasourceName, ProjectLUID: o.Result.ProjectLUID, OwnerLUID: o.Result.OwnerLUID}
	}
	return updateCompactOutput{Plan: o.Plan, Result: r, Details: "--full", Help: o.Help}
}
func (o UpdateOutput) FullOutput() any { return o }

type updateDatasource = value.OwnedContentIdentity
