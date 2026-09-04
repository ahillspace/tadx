package get

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

const detailLimit = 50

// Input selects one authoritative workbook.
type Input struct {
	Environment, Site string
	Selector          identity.Selector
	Catalog           bool
}

// SetSelector records one exact CLI selector.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// Workbook is one bounded workbook projection.
type Workbook struct {
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

// Output retains full details before projection.
type Output struct {
	Status, Environment, Site string
	Workbook                  Workbook
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}

// CompactWorkbook identifies the authoritative target.
type CompactWorkbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	ContentURL  string `json:"content_url,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Workbook    CompactWorkbook      `json:"workbook"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the bounded expanded projection.
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Workbook    Workbook             `json:"workbook"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns selected fields.
func (o Output) CompactOutput() any {
	item := CompactWorkbook{LUID: o.Workbook.LUID, Name: o.Workbook.Name, ProjectLUID: o.Workbook.ProjectLUID, ProjectPath: o.Workbook.ProjectPath, ContentURL: o.Workbook.ContentURL, UpdatedAt: o.Workbook.UpdatedAt}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Workbook: item, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all bounded fields.
func (o Output) FullOutput() any {
	item := o.Workbook
	item.Tags = append([]string(nil), item.Tags...)
	if len(item.Tags) > detailLimit {
		item.TagsOmitted = len(item.Tags) - detailLimit
		item.Tags = item.Tags[:detailLimit]
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Workbook: item, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
