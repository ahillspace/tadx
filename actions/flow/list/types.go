package list

import (
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

const fullTagsPerFlowLimit = 50

// Input selects one flow page.
type Input struct {
	All                                                                  bool
	Environment, Site, Cursor, Name, OwnerName, ProjectLUID, ProjectName string
	Limit                                                                int
	Catalog                                                              bool
}

// PageRequest is the action-owned request.
type PageRequest struct {
	PageNumber, PageSize                      int
	Name, OwnerName, ProjectLUID, ProjectName string
	SnapshotCursor                            string
}

// Flow is one complete lifecycle projection.
type Flow struct {
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

// Page is one complete reader page.
type Page struct {
	Number, Size, Total  int
	Flows                []Flow
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}

// OutputPage is continuation metadata.
type OutputPage = output.Page

// Output retains details before projection.
type Output struct {
	Status, Environment, Site string
	Page                      OutputPage
	Flows                     []Flow
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}

// CompactFlow identifies one lifecycle resource.
type CompactFlow struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectName string `json:"project_name"`
	FileType    string `json:"file_type"`
	UpdatedAt   string `json:"updated_at"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Flows       []CompactFlow        `json:"flows"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the current expanded page.
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Flows       []Flow               `json:"flows"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns selected fields.
func (o Output) CompactOutput() any {
	items := make([]CompactFlow, len(o.Flows))
	for i, item := range o.Flows {
		items[i] = CompactFlow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, FileType: item.FileType, UpdatedAt: item.UpdatedAt}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Flows: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all bounded current-page fields.
func (o Output) FullOutput() any {
	flows := append([]Flow(nil), o.Flows...)
	for index := range flows {
		flows[index].Tags = append([]string(nil), flows[index].Tags...)
		if len(flows[index].Tags) > fullTagsPerFlowLimit {
			flows[index].TagsOmitted = len(flows[index].Tags) - fullTagsPerFlowLimit
			flows[index].Tags = flows[index].Tags[:fullTagsPerFlowLimit]
		}
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Flows: flows, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
