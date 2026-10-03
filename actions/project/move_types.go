package project

import "github.com/ahillspace/tadx/internal/identity"

type MoveInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                  bool
	Environment, Site               string
	ProjectSelector, ParentSelector identity.Selector
	TopLevel                        bool
}

func (i *MoveInput) SetProjectSelector(luid, path string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: path}
}
func (i *MoveInput) SetParentSelector(luid, path string) {
	i.ParentSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: path}
}

type MoveProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	PathUnavailableReason           string `json:"path_unavailable_reason,omitempty"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}
type MovePlan struct {
	Mode        string       `json:"mode"`
	Operation   string       `json:"operation"`
	Environment string       `json:"environment"`
	Site        string       `json:"site"`
	Source      MoveProject  `json:"source"`
	Destination *MoveProject `json:"destination,omitempty"`
	TopLevel    bool         `json:"top_level"`
	NoOp        bool         `json:"no_op"`
}
type MoveResult struct {
	Status           string      `json:"status"`
	Project          MoveProject `json:"project"`
	TableauRequestID string      `json:"tableau_request_id,omitempty"`
}
type MoveOutput struct {
	Plan   MovePlan    `json:"plan"`
	Result *MoveResult `json:"result,omitempty"`
	Help   []string    `json:"help"`
}
type moveCompactResult struct {
	Status  string      `json:"status"`
	Project MoveProject `json:"project"`
}
type moveCompactOutput struct {
	Plan    MovePlan           `json:"plan"`
	Result  *moveCompactResult `json:"result,omitempty"`
	Details string             `json:"details"`
	Help    []string           `json:"help"`
}

func (o MoveOutput) CompactOutput() any {
	var r *moveCompactResult
	if o.Result != nil {
		r = &moveCompactResult{Status: o.Result.Status, Project: o.Result.Project}
	}
	return moveCompactOutput{Plan: o.Plan, Result: r, Details: "--full", Help: o.Help}
}
func (o MoveOutput) FullOutput() any { return o }
