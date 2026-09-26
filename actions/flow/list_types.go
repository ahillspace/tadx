package flow

import (
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

const listFullTagsPerFlowLimit = 50

// ListInput selects one flow page.
type ListInput struct {
	All                                                                  bool
	Environment, Site, Cursor, Name, OwnerName, ProjectLUID, ProjectName string
	Limit                                                                int
	Cache                                                                bool
}

// ListPageRequest is the action-owned request.
type ListPageRequest struct {
	PageNumber, PageSize                      int
	Name, OwnerName, ProjectLUID, ProjectName string
	SnapshotCursor                            string
}

// listFlow preserves the expanded list projection.
type listFlow struct {
	LUID        string   `json:"luid"`
	Name        string   `json:"name"`
	ProjectLUID string   `json:"project_luid"`
	ProjectName string   `json:"project_name,omitempty"`
	ProjectPath string   `json:"project_path,omitempty"`
	FileType    string   `json:"file_type,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Description string   `json:"description,omitempty"`
	OwnerLUID   string   `json:"owner_luid,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	TagsOmitted int      `json:"tags_omitted,omitempty"`
}

// ListPage is one complete reader page.
type ListPage struct {
	Number, Size, Total  int
	Flows                []Record
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}

// ListOutputPage is continuation metadata.
type ListOutputPage = output.Page

// ListOutput retains details before projection.
type ListOutput struct {
	Status, Environment, Site string
	Page                      ListOutputPage
	Flows                     []Record
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}

// CompactFlow identifies one lifecycle resource.
type ListCompactFlow struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectName string `json:"project_name"`
	FileType    string `json:"file_type"`
	UpdatedAt   string `json:"updated_at"`
}

// CompactResult is the default projection.
type ListCompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Flows       []ListCompactFlow    `json:"flows"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the current expanded page.
type ListFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Flows       []listFlow           `json:"flows"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns selected fields.
func (o ListOutput) CompactOutput() any {
	items := make([]ListCompactFlow, len(o.Flows))
	for i, item := range o.Flows {
		items[i] = ListCompactFlow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, FileType: item.FileType, UpdatedAt: item.UpdatedAt}
	}
	return ListCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Flows: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all bounded current-page fields.
func (o ListOutput) FullOutput() any {
	flows := make([]listFlow, len(o.Flows))
	for index, item := range o.Flows {
		flows[index] = listFlow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: item.ProjectPath, FileType: item.FileType, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...)}
		if len(flows[index].Tags) > listFullTagsPerFlowLimit {
			flows[index].TagsOmitted = len(flows[index].Tags) - listFullTagsPerFlowLimit
			flows[index].Tags = flows[index].Tags[:listFullTagsPerFlowLimit]
		}
	}
	return ListFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Flows: flows, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
