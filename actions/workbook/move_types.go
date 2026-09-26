package workbook

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type MoveInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                    bool
	Environment, Site                 string
	WorkbookSelector, ProjectSelector identity.Selector
}

func (i *MoveInput) SetWorkbookSelector(luid, name, projectPath string) {
	i.WorkbookSelector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

func (i *MoveInput) SetProjectSelector(luid, projectPath string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

type MovePlan struct {
	Mode        string       `json:"mode"`
	Operation   string       `json:"operation"`
	Environment string       `json:"environment"`
	Site        string       `json:"site"`
	Source      moveWorkbook `json:"source"`
	Destination Project      `json:"destination"`
	NoOp        bool         `json:"no_op"`
}
type MoveResult struct {
	Status           string `json:"status"`
	WorkbookLUID     string `json:"workbook_luid"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type MoveOutput struct {
	Plan   MovePlan    `json:"plan"`
	Result *MoveResult `json:"result,omitempty"`
	Help   []string    `json:"help"`
}
type moveCompactResult struct {
	Status       string `json:"status"`
	WorkbookLUID string `json:"workbook_luid"`
	ProjectLUID  string `json:"project_luid"`
}
type moveCompactOutput struct {
	Plan    MovePlan           `json:"plan"`
	Result  *moveCompactResult `json:"result,omitempty"`
	Details string             `json:"details"`
	Help    []string           `json:"help"`
}

func (o MoveOutput) CompactOutput() any {
	result := (*moveCompactResult)(nil)
	if o.Result != nil {
		result = &moveCompactResult{Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, ProjectLUID: o.Result.ProjectLUID}
	}
	return moveCompactOutput{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o MoveOutput) FullOutput() any { return o }

type moveWorkbook = value.OwnedContentIdentity
