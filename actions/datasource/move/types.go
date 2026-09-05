package move

import "github.com/ahillspace/tadx/internal/identity"

type Input struct {
	Environment, Site                   string
	DatasourceSelector, ProjectSelector identity.Selector
}

func (i *Input) SetDatasourceSelector(luid, name, projectPath string) {
	i.DatasourceSelector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}
func (i *Input) SetProjectSelector(luid, path string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: path}
}

type Datasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	OwnerLUID   string `json:"owner_luid"`
}
type Project struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
	Path string `json:"path"`
}
type Plan struct {
	Mode        string     `json:"mode"`
	Operation   string     `json:"operation"`
	Environment string     `json:"environment"`
	Site        string     `json:"site"`
	Source      Datasource `json:"source"`
	Destination Project    `json:"destination"`
	NoOp        bool       `json:"no_op"`
}
type Result struct {
	Status           string `json:"status"`
	DatasourceLUID   string `json:"datasource_luid"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type compactResult struct {
	Status         string `json:"status"`
	DatasourceLUID string `json:"datasource_luid"`
	ProjectLUID    string `json:"project_luid"`
}
type compactOutput struct {
	Plan    Plan           `json:"plan"`
	Result  *compactResult `json:"result,omitempty"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

func (o Output) CompactOutput() any {
	var r *compactResult
	if o.Result != nil {
		r = &compactResult{Status: o.Result.Status, DatasourceLUID: o.Result.DatasourceLUID, ProjectLUID: o.Result.ProjectLUID}
	}
	return compactOutput{Plan: o.Plan, Result: r, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
