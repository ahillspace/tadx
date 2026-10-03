package project

import (
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

// Input selects one bounded project page.
type ListInput struct {
	All         bool
	Environment string
	Site        string
	Cursor      string
	Limit       int
	Name        string
	ParentLUID  string
	OwnerName   string
	TopLevel    *bool
	Cache       bool
}

// PageRequest is the action-owned read request.
type ListPageRequest struct {
	PageNumber     int
	PageSize       int
	Name           string
	ParentLUID     string
	OwnerName      string
	TopLevel       *bool
	SnapshotCursor string
}

// Project is one complete project projection.
type ListProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	ParentLUID                      string `json:"parent_luid"`
	Description                     string `json:"description"`
	OwnerLUID                       string `json:"owner_luid,omitempty"`
	TopLevel                        *bool  `json:"top_level,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
	CreatedAt                       string `json:"created_at,omitempty"`
	UpdatedAt                       string `json:"updated_at,omitempty"`
	ProjectCount                    *int   `json:"project_count,omitempty"`
	WorkbookCount                   *int   `json:"workbook_count,omitempty"`
	ViewCount                       *int   `json:"view_count,omitempty"`
	DatasourceCount                 *int   `json:"datasource_count,omitempty"`
}

// Page is one complete page returned by the reader.
type ListPage struct {
	Number               int
	Size                 int
	Total                int
	Projects             []ListProject
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}

// OutputPage is bounded continuation metadata.
type ListOutputPage = output.Page

// Output is the complete result before projection.
type ListOutput struct {
	Status      string
	Environment string
	Site        string
	Page        ListOutputPage
	Projects    []ListProject
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// CompactProject identifies one project and its direct parent.
type ListCompactProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	ParentLUID                      string `json:"parent_luid"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// CompactResult is the default bounded projection.
type ListCompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Projects    []ListCompactProject `json:"projects"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the bounded expanded current page.
type ListFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Projects    []ListProject        `json:"projects"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns explicit compact fields.
func (o ListOutput) CompactOutput() any {
	projects := make([]ListCompactProject, len(o.Projects))
	for index, project := range o.Projects {
		projects[index] = ListCompactProject{LUID: project.LUID, Name: project.Name, ParentLUID: project.ParentLUID, ContentPermissions: project.ContentPermissions, ControllingPermissionsProjectID: project.ControllingPermissionsProjectID}
	}
	return ListCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Projects: projects, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns the same page with bounded lifecycle fields.
func (o ListOutput) FullOutput() any {
	projects := make([]ListProject, len(o.Projects))
	copy(projects, o.Projects)
	for index := range projects {
		if projects[index].TopLevel == nil && projects[index].ParentLUID == "" {
			projects[index].TopLevel = new(true)
		}
	}
	return ListFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Projects: projects, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
