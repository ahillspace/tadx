package list

import "github.com/ahillspace/tadx/internal/readsource"

// Input selects one bounded project page.
type Input struct {
	Environment string
	Site        string
	Cursor      string
	Limit       int
	Name        string
	ParentLUID  string
	OwnerName   string
	TopLevel    *bool
	Catalog     bool
}

// PageRequest is the action-owned read request.
type PageRequest struct {
	PageNumber int
	PageSize   int
	Name       string
	ParentLUID string
	OwnerName  string
	TopLevel   *bool
}

// Project is one complete project projection.
type Project struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	Description                     string `json:"description,omitempty"`
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
type Page struct {
	Number    int
	Size      int
	Total     int
	Projects  []Project
	RequestID string
}

// OutputPage is bounded continuation metadata.
type OutputPage struct {
	Returned   int    `json:"returned"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Output is the complete result before projection.
type Output struct {
	Status      string
	Environment string
	Site        string
	Page        OutputPage
	Projects    []Project
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// CompactProject identifies one project and its direct parent.
type CompactProject struct {
	LUID       string `json:"luid"`
	Name       string `json:"name"`
	ParentLUID string `json:"parent_luid,omitempty"`
}

// CompactResult is the default bounded projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Projects    []CompactProject     `json:"projects"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the bounded expanded current page.
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Projects    []Project            `json:"projects"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns explicit compact fields.
func (o Output) CompactOutput() any {
	projects := make([]CompactProject, len(o.Projects))
	for index, project := range o.Projects {
		projects[index] = CompactProject{LUID: project.LUID, Name: project.Name, ParentLUID: project.ParentLUID}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Projects: projects, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns the same page with bounded lifecycle fields.
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Projects: o.Projects, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
