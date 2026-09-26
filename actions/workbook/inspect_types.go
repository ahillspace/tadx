package workbook

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

const inspectDetailLimit = 50

// InspectInput selects one authoritative workbook.
type InspectInput struct {
	Environment, Site string
	Selector          identity.Selector
	Cache             bool
}

// SetSelector records one exact CLI selector.
func (i *InspectInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// SetSelectorWithProjectLUID records an authoritative or exact selector using a project LUID.
func (i *InspectInput) SetSelectorWithProjectLUID(luid, name, projectPath, projectLUID string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath, ProjectLUID: identity.LUID(projectLUID)}
}

// InspectOutput retains full details before projection.
type InspectOutput struct {
	Status, Environment, Site string
	Workbook                  Record
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}

// InspectCompactWorkbook identifies the authoritative target.
type InspectCompactWorkbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	ContentURL  string `json:"content_url,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// InspectCompactResult is the default projection.
type InspectCompactResult struct {
	Status      string                 `json:"status"`
	Environment string                 `json:"environment,omitempty"`
	Site        string                 `json:"site,omitempty"`
	Workbook    InspectCompactWorkbook `json:"workbook"`
	Details     string                 `json:"details"`
	Help        []string               `json:"help"`
	Source      *readsource.Metadata   `json:"source,omitempty"`
}

// InspectFullResult is the bounded expanded projection.
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Workbook    inspectWorkbook      `json:"workbook"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns selected fields.
func (o InspectOutput) CompactOutput() any {
	item := InspectCompactWorkbook{LUID: o.Workbook.LUID, Name: o.Workbook.Name, ProjectLUID: o.Workbook.ProjectLUID, ProjectPath: o.Workbook.ProjectPath, ContentURL: o.Workbook.ContentURL, UpdatedAt: o.Workbook.UpdatedAt}
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Workbook: item, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all bounded fields.
func (o InspectOutput) FullOutput() any {
	w := o.Workbook
	item := inspectWorkbook{LUID: w.LUID, Name: w.Name, ProjectLUID: w.ProjectLUID, ProjectPath: w.ProjectPath, ContentURL: w.ContentURL, UpdatedAt: w.UpdatedAt, Description: w.Description, OwnerLUID: w.OwnerLUID, CreatedAt: w.CreatedAt, Tags: append([]string(nil), w.Tags...), RequestID: w.RequestID}
	if len(item.Tags) > inspectDetailLimit {
		item.TagsOmitted = len(item.Tags) - inspectDetailLimit
		item.Tags = item.Tags[:inspectDetailLimit]
	}
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Workbook: item, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type inspectWorkbook struct {
	LUID        string   `json:"luid"`
	Name        string   `json:"name"`
	ProjectLUID string   `json:"project_luid"`
	ProjectPath string   `json:"project_path"`
	ContentURL  string   `json:"content_url,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Description string   `json:"description,omitempty"`
	OwnerLUID   string   `json:"owner_luid,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	TagsOmitted int      `json:"tags_omitted,omitempty"`
	RequestID   string   `json:"-"`
}
