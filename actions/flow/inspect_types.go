package flow

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
)

const inspectDetailLimit = 50

type InspectInput struct {
	Environment, Site string
	Selector          identity.Selector
	Cache             bool
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *InspectInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// SetSelectorWithProjectLUID records an authoritative or exact selector using a project LUID.
func (i *InspectInput) SetSelectorWithProjectLUID(luid, name, projectPath, projectLUID string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath, ProjectLUID: identity.LUID(projectLUID)}
}

type InspectParameter = value.FlowParameter
type InspectOutputStep = value.FlowOutputStep

// InspectFlow preserves the expanded inspection and unbounded cache projection.
type InspectFlow struct {
	LUID               string              `json:"luid"`
	Name               string              `json:"name"`
	ProjectLUID        string              `json:"project_luid"`
	ProjectPath        string              `json:"project_path"`
	FileType           string              `json:"file_type,omitempty"`
	UpdatedAt          string              `json:"updated_at,omitempty"`
	Description        string              `json:"description,omitempty"`
	OwnerLUID          string              `json:"owner_luid,omitempty"`
	CreatedAt          string              `json:"created_at,omitempty"`
	Tags               []string            `json:"tags,omitempty"`
	Parameters         []InspectParameter  `json:"parameters,omitempty"`
	OutputSteps        []InspectOutputStep `json:"output_steps,omitempty"`
	TagsOmitted        int                 `json:"tags_omitted,omitempty"`
	ParametersOmitted  int                 `json:"parameters_omitted,omitempty"`
	OutputStepsOmitted int                 `json:"output_steps_omitted,omitempty"`
	RequestID          string              `json:"-"`
}

type InspectOutput struct {
	Status, Environment, Site string
	Flow                      Record
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type InspectCompactFlow struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	FileType    string `json:"file_type,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}
type InspectCompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Flow        InspectCompactFlow   `json:"flow"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Flow        InspectFlow          `json:"flow"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o InspectOutput) CompactOutput() any {
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Flow: InspectCompactFlow{LUID: o.Flow.LUID, Name: o.Flow.Name, ProjectLUID: o.Flow.ProjectLUID, ProjectPath: o.Flow.ProjectPath, FileType: o.Flow.FileType, UpdatedAt: o.Flow.UpdatedAt}, Details: "--full", Help: o.Help, Source: o.Source}
}

// CacheFlow preserves the original unbounded detail-cache payload.
func (o InspectOutput) CacheFlow() InspectFlow {
	item := o.Flow
	return InspectFlow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...), Parameters: append([]InspectParameter(nil), item.Parameters...), OutputSteps: append([]InspectOutputStep(nil), item.OutputSteps...), RequestID: item.RequestID}
}
func (o InspectOutput) FullOutput() any {
	flow := o.CacheFlow()
	if len(flow.Tags) > inspectDetailLimit {
		flow.TagsOmitted = len(flow.Tags) - inspectDetailLimit
		flow.Tags = flow.Tags[:inspectDetailLimit]
	}
	if len(flow.Parameters) > inspectDetailLimit {
		flow.ParametersOmitted = len(flow.Parameters) - inspectDetailLimit
		flow.Parameters = flow.Parameters[:inspectDetailLimit]
	}
	if len(flow.OutputSteps) > inspectDetailLimit {
		flow.OutputStepsOmitted = len(flow.OutputSteps) - inspectDetailLimit
		flow.OutputSteps = flow.OutputSteps[:inspectDetailLimit]
	}
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Flow: flow, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
