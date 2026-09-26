package workbook

import (
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

const listFullTagsPerWorkbookLimit = 50

// ListInput selects one workbook page and its exact upstream filters.
type ListInput struct {
	All                                                                       bool
	Environment, Site, Cursor, Name, OwnerName, ProjectLUID, ProjectName, Tag string
	Limit                                                                     int
	Cache                                                                     bool
}

// ListPageRequest is the action-owned request.
type ListPageRequest struct {
	PageNumber, PageSize                           int
	Name, OwnerName, ProjectLUID, ProjectName, Tag string
	SnapshotCursor                                 string
}

// ListPage is one complete reader page.
type ListPage struct {
	Number, Size, Total  int
	Workbooks            []Record
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}

// ListOutputPage is continuation metadata.
type ListOutputPage = output.Page

// ListOutput retains full details before projection.
type ListOutput struct {
	Status, Environment, Site string
	Page                      ListOutputPage
	Workbooks                 []Record
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}

// ListCompactWorkbook identifies one lifecycle resource.
type ListCompactWorkbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	ContentURL  string `json:"content_url"`
	UpdatedAt   string `json:"updated_at"`
}

// ListCompactResult is the default projection.
type ListCompactResult struct {
	Status      string                `json:"status"`
	Environment string                `json:"environment,omitempty"`
	Site        string                `json:"site,omitempty"`
	Page        ListOutputPage        `json:"page"`
	Workbooks   []ListCompactWorkbook `json:"workbooks"`
	Details     string                `json:"details"`
	Help        []string              `json:"help"`
	Source      *readsource.Metadata  `json:"source,omitempty"`
}

// ListFullResult is the bounded expanded page.
type ListFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Workbooks   []listWorkbook       `json:"workbooks"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns selected fields.
func (o ListOutput) CompactOutput() any {
	items := make([]ListCompactWorkbook, len(o.Workbooks))
	for index, item := range o.Workbooks {
		items[index] = ListCompactWorkbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt}
	}
	return ListCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Workbooks: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all bounded current-page fields.
func (o ListOutput) FullOutput() any {
	items := make([]listWorkbook, len(o.Workbooks))
	for index, item := range o.Workbooks {
		items[index] = listWorkbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...)}
		if len(items[index].Tags) > listFullTagsPerWorkbookLimit {
			items[index].TagsOmitted = len(items[index].Tags) - listFullTagsPerWorkbookLimit
			items[index].Tags = items[index].Tags[:listFullTagsPerWorkbookLimit]
		}
	}
	return ListFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Workbooks: items, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type listWorkbook struct {
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
