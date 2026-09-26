package workbook

import (
	"github.com/ahillspace/tadx/internal/identity"
)

type UpdateInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved               bool
	Environment, Site            string
	Selector                     identity.Selector
	Name, OwnerLUID, Description *string
}

func (i *UpdateInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

type UpdateRequest struct {
	LUID                         string
	Name, OwnerLUID, Description *string
}
type UpdateChange struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type UpdatePlan struct {
	Mode        string         `json:"mode"`
	Operation   string         `json:"operation"`
	Environment string         `json:"environment"`
	Site        string         `json:"site"`
	Target      updateWorkbook `json:"target"`
	Changes     []UpdateChange `json:"changes"`
	NoOp        bool           `json:"no_op"`
}
type UpdateResult struct {
	Status           string  `json:"status"`
	WorkbookLUID     string  `json:"workbook_luid"`
	WorkbookName     string  `json:"workbook_name"`
	ProjectLUID      string  `json:"project_luid"`
	OwnerLUID        string  `json:"owner_luid"`
	Description      *string `json:"description,omitempty"`
	EvidenceSource   string  `json:"evidence_source,omitempty"`
	TableauRequestID string  `json:"tableau_request_id,omitempty"`
}
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type updateCompactResult struct {
	Status         string  `json:"status"`
	WorkbookLUID   string  `json:"workbook_luid"`
	WorkbookName   string  `json:"workbook_name"`
	ProjectLUID    string  `json:"project_luid"`
	OwnerLUID      string  `json:"owner_luid"`
	Description    *string `json:"description,omitempty"`
	EvidenceSource string  `json:"evidence_source,omitempty"`
}
type updateCompactOutput struct {
	Plan    UpdatePlan           `json:"plan"`
	Result  *updateCompactResult `json:"result,omitempty"`
	Details string               `json:"details"`
	Help    []string             `json:"help"`
}

func (o UpdateOutput) CompactOutput() any {
	var result *updateCompactResult
	if o.Result != nil {
		result = &updateCompactResult{Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, WorkbookName: o.Result.WorkbookName, ProjectLUID: o.Result.ProjectLUID, OwnerLUID: o.Result.OwnerLUID, Description: o.Result.Description, EvidenceSource: o.Result.EvidenceSource}
	}
	return updateCompactOutput{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o UpdateOutput) FullOutput() any { return o }

type updateWorkbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	OwnerLUID   string `json:"owner_luid"`
	Description string `json:"description,omitempty"`
}
