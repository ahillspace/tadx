package inspect

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

const detailLimit = 50

type Input struct {
	Environment, Site string
	Selector          identity.Selector
	Cache             bool
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// SetSelectorWithProjectLUID records an authoritative or exact selector using a project LUID.
func (i *Input) SetSelectorWithProjectLUID(luid, name, projectPath, projectLUID string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath, ProjectLUID: identity.LUID(projectLUID)}
}

type Parameter struct {
	LUID        string `json:"luid,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value,omitempty"`
	Required    *bool  `json:"required,omitempty"`
}
type OutputStep struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
}
type Flow struct {
	LUID               string       `json:"luid"`
	Name               string       `json:"name"`
	ProjectLUID        string       `json:"project_luid"`
	ProjectPath        string       `json:"project_path"`
	FileType           string       `json:"file_type,omitempty"`
	UpdatedAt          string       `json:"updated_at,omitempty"`
	Description        string       `json:"description,omitempty"`
	OwnerLUID          string       `json:"owner_luid,omitempty"`
	CreatedAt          string       `json:"created_at,omitempty"`
	Tags               []string     `json:"tags,omitempty"`
	Parameters         []Parameter  `json:"parameters,omitempty"`
	OutputSteps        []OutputStep `json:"output_steps,omitempty"`
	TagsOmitted        int          `json:"tags_omitted,omitempty"`
	ParametersOmitted  int          `json:"parameters_omitted,omitempty"`
	OutputStepsOmitted int          `json:"output_steps_omitted,omitempty"`
	RequestID          string       `json:"-"`
}
type Output struct {
	Status, Environment, Site string
	Flow                      Flow
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type CompactFlow struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	FileType    string `json:"file_type,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Flow        CompactFlow          `json:"flow"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Flow        Flow                 `json:"flow"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Flow: CompactFlow{LUID: o.Flow.LUID, Name: o.Flow.Name, ProjectLUID: o.Flow.ProjectLUID, ProjectPath: o.Flow.ProjectPath, FileType: o.Flow.FileType, UpdatedAt: o.Flow.UpdatedAt}, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any {
	flow := o.Flow
	flow.Tags = append([]string(nil), flow.Tags...)
	flow.Parameters = append([]Parameter(nil), flow.Parameters...)
	flow.OutputSteps = append([]OutputStep(nil), flow.OutputSteps...)
	if len(flow.Tags) > detailLimit {
		flow.TagsOmitted = len(flow.Tags) - detailLimit
		flow.Tags = flow.Tags[:detailLimit]
	}
	if len(flow.Parameters) > detailLimit {
		flow.ParametersOmitted = len(flow.Parameters) - detailLimit
		flow.Parameters = flow.Parameters[:detailLimit]
	}
	if len(flow.OutputSteps) > detailLimit {
		flow.OutputStepsOmitted = len(flow.OutputSteps) - detailLimit
		flow.OutputSteps = flow.OutputSteps[:detailLimit]
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Flow: flow, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
