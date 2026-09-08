package list

import (
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

const fullTagsPerWorkbookLimit = 50

// Input selects one workbook page and its exact upstream filters.
type Input struct {
	All                                                          bool
	Environment, Site, Cursor, Name, OwnerName, ProjectName, Tag string
	Limit                                                        int
	Catalog                                                      bool
}

// PageRequest is the action-owned request.
type PageRequest struct {
	PageNumber, PageSize              int
	Name, OwnerName, ProjectName, Tag string
	SnapshotCursor                    string
}

// Workbook is one complete bounded lifecycle projection.
type Workbook struct {
	LUID        string   `json:"luid"`
	Name        string   `json:"name"`
	ProjectLUID string   `json:"project_luid"`
	ProjectPath string   `json:"project_path,omitempty"`
	ContentURL  string   `json:"content_url,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Description string   `json:"description,omitempty"`
	OwnerLUID   string   `json:"owner_luid,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	TagsOmitted int      `json:"tags_omitted,omitempty"`
}

// Page is one complete reader page.
type Page struct {
	Number, Size, Total  int
	Workbooks            []Workbook
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}

// OutputPage is continuation metadata.
type OutputPage = output.Page

// Output retains full details before projection.
type Output struct {
	Status, Environment, Site string
	Page                      OutputPage
	Workbooks                 []Workbook
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}

// CompactWorkbook identifies one lifecycle resource.
type CompactWorkbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	ContentURL  string `json:"content_url"`
	UpdatedAt   string `json:"updated_at"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Workbooks   []CompactWorkbook    `json:"workbooks"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the bounded expanded page.
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Workbooks   []Workbook           `json:"workbooks"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns selected fields.
func (o Output) CompactOutput() any {
	items := make([]CompactWorkbook, len(o.Workbooks))
	for index, item := range o.Workbooks {
		items[index] = CompactWorkbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Workbooks: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all bounded current-page fields.
func (o Output) FullOutput() any {
	items := append([]Workbook(nil), o.Workbooks...)
	for index := range items {
		items[index].Tags = append([]string(nil), items[index].Tags...)
		if len(items[index].Tags) > fullTagsPerWorkbookLimit {
			items[index].TagsOmitted = len(items[index].Tags) - fullTagsPerWorkbookLimit
			items[index].Tags = items[index].Tags[:fullTagsPerWorkbookLimit]
		}
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Workbooks: items, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
