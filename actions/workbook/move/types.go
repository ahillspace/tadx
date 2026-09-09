package move

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                    bool
	Environment, Site                 string
	WorkbookSelector, ProjectSelector identity.Selector
}

func (i *Input) SetWorkbookSelector(luid, name, projectPath string) {
	i.WorkbookSelector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

func (i *Input) SetProjectSelector(luid, projectPath string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

type Workbook = value.OwnedContentIdentity
type Project = value.ProjectIdentity
type Plan struct {
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	Source      Workbook `json:"source"`
	Destination Project  `json:"destination"`
	NoOp        bool     `json:"no_op"`
}
type Result struct {
	Status           string `json:"status"`
	WorkbookLUID     string `json:"workbook_luid"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type compactResult struct {
	Status       string `json:"status"`
	WorkbookLUID string `json:"workbook_luid"`
	ProjectLUID  string `json:"project_luid"`
}
type compactOutput struct {
	Plan    Plan           `json:"plan"`
	Result  *compactResult `json:"result,omitempty"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

func (o Output) CompactOutput() any {
	result := (*compactResult)(nil)
	if o.Result != nil {
		result = &compactResult{Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, ProjectLUID: o.Result.ProjectLUID}
	}
	return compactOutput{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
