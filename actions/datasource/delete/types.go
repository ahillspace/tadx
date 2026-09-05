package delete

import "github.com/ahillspace/tadx/internal/identity"

type Input struct {
	Environment, Site string
	Selector          identity.Selector
}

func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

type Datasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
}

type Plan struct {
	Mode        string     `json:"mode"`
	Operation   string     `json:"operation"`
	Environment string     `json:"environment"`
	Site        string     `json:"site"`
	Target      Datasource `json:"target"`
}

type Result struct {
	Status           string `json:"status"`
	DatasourceLUID   string `json:"datasource_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

type CompactResultValue struct {
	Status         string `json:"status"`
	DatasourceLUID string `json:"datasource_luid"`
}

type CompactResult struct {
	Plan    Plan                `json:"plan"`
	Result  *CompactResultValue `json:"result,omitempty"`
	Details string              `json:"details"`
	Help    []string            `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactResultValue
	if o.Result != nil {
		result = &CompactResultValue{Status: o.Result.Status, DatasourceLUID: o.Result.DatasourceLUID}
	}
	return CompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

func (o Output) FullOutput() any { return o }
