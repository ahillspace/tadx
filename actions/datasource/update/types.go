package update

import "github.com/ahillspace/tadx/internal/identity"

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved    bool
	Environment, Site string
	Selector          identity.Selector
	Name, OwnerLUID   *string
}

func (i *Input) SetSelector(luid, name, path string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: path}
}

type Datasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	OwnerLUID   string `json:"owner_luid"`
}
type Request struct {
	LUID            string
	Name, OwnerLUID *string
}
type Change struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type Plan struct {
	Mode        string     `json:"mode"`
	Operation   string     `json:"operation"`
	Environment string     `json:"environment"`
	Site        string     `json:"site"`
	Target      Datasource `json:"target"`
	Changes     []Change   `json:"changes"`
	NoOp        bool       `json:"no_op"`
}
type Result struct {
	Status           string `json:"status"`
	DatasourceLUID   string `json:"datasource_luid"`
	DatasourceName   string `json:"datasource_name"`
	ProjectLUID      string `json:"project_luid"`
	OwnerLUID        string `json:"owner_luid"`
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
	DatasourceName string `json:"datasource_name"`
	ProjectLUID    string `json:"project_luid"`
	OwnerLUID      string `json:"owner_luid"`
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
		r = &compactResult{Status: o.Result.Status, DatasourceLUID: o.Result.DatasourceLUID, DatasourceName: o.Result.DatasourceName, ProjectLUID: o.Result.ProjectLUID, OwnerLUID: o.Result.OwnerLUID}
	}
	return compactOutput{Plan: o.Plan, Result: r, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
