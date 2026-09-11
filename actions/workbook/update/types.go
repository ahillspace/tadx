package update

import (
	"github.com/ahillspace/tadx/internal/identity"
)

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved               bool
	Environment, Site            string
	Selector                     identity.Selector
	Name, OwnerLUID, Description *string
}

func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

type Workbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	OwnerLUID   string `json:"owner_luid"`
	Description string `json:"description,omitempty"`
}
type Request struct {
	LUID                         string
	Name, OwnerLUID, Description *string
}
type Change struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type Plan struct {
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	Target      Workbook `json:"target"`
	Changes     []Change `json:"changes"`
	NoOp        bool     `json:"no_op"`
}
type Result struct {
	Status           string `json:"status"`
	WorkbookLUID     string `json:"workbook_luid"`
	WorkbookName     string `json:"workbook_name"`
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
	Status       string `json:"status"`
	WorkbookLUID string `json:"workbook_luid"`
	WorkbookName string `json:"workbook_name"`
	ProjectLUID  string `json:"project_luid"`
	OwnerLUID    string `json:"owner_luid"`
}
type compactOutput struct {
	Plan    Plan           `json:"plan"`
	Result  *compactResult `json:"result,omitempty"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *compactResult
	if o.Result != nil {
		result = &compactResult{Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, WorkbookName: o.Result.WorkbookName, ProjectLUID: o.Result.ProjectLUID, OwnerLUID: o.Result.OwnerLUID}
	}
	return compactOutput{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
