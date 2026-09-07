package move

import "github.com/ahillspace/tadx/internal/identity"

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                  bool
	Environment, Site               string
	ProjectSelector, ParentSelector identity.Selector
	TopLevel                        bool
}

func (i *Input) SetProjectSelector(luid, path string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: path}
}
func (i *Input) SetParentSelector(luid, path string) {
	i.ParentSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: path}
}

type Project struct {
	LUID       string `json:"luid"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	ParentLUID string `json:"parent_luid,omitempty"`
}
type Plan struct {
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	Source      Project  `json:"source"`
	Destination *Project `json:"destination,omitempty"`
	TopLevel    bool     `json:"top_level"`
	NoOp        bool     `json:"no_op"`
}
type Result struct {
	Status           string  `json:"status"`
	Project          Project `json:"project"`
	TableauRequestID string  `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type compactResult struct {
	Status  string  `json:"status"`
	Project Project `json:"project"`
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
		r = &compactResult{Status: o.Result.Status, Project: o.Result.Project}
	}
	return compactOutput{Plan: o.Plan, Result: r, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
