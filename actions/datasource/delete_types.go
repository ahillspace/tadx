package datasource

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

func (i *DeleteInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

type DeletePlan struct {
	Mode        string           `json:"mode"`
	Operation   string           `json:"operation"`
	Environment string           `json:"environment"`
	Site        string           `json:"site"`
	Target      deleteDatasource `json:"target"`
}

type DeleteResult struct {
	Status           string `json:"status"`
	DatasourceLUID   string `json:"datasource_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

type DeleteOutput struct {
	Plan   DeletePlan    `json:"plan"`
	Result *DeleteResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

type DeleteCompactResultValue struct {
	Status         string `json:"status"`
	DatasourceLUID string `json:"datasource_luid"`
}

type DeleteCompactResult struct {
	Plan    DeletePlan                `json:"plan"`
	Result  *DeleteCompactResultValue `json:"result,omitempty"`
	Details string                    `json:"details"`
	Help    []string                  `json:"help"`
}

func (o DeleteOutput) CompactOutput() any {
	var result *DeleteCompactResultValue
	if o.Result != nil {
		result = &DeleteCompactResultValue{Status: o.Result.Status, DatasourceLUID: o.Result.DatasourceLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

func (o DeleteOutput) FullOutput() any { return o }

type deleteDatasource = value.ContentIdentity
