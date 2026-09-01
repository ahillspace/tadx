package get

import "github.com/ahillspace/tadx/internal/identity"

// Input selects one exact project.
type Input struct {
	Environment string
	Site        string
	Selector    identity.Selector
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *Input) SetSelector(luid, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

// Project is one complete project projection.
type Project struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
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
	RequestID                       string `json:"-"`
}

// Output retains complete details before projection.
type Output struct {
	Status      string
	Environment string
	Site        string
	Project     Project
	RequestID   string
	Help        []string
}

// CompactProject is the exact identity needed for another action.
type CompactProject struct {
	LUID       string `json:"luid"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	ParentLUID string `json:"parent_luid,omitempty"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Status      string         `json:"status"`
	Environment string         `json:"environment,omitempty"`
	Site        string         `json:"site,omitempty"`
	Project     CompactProject `json:"project"`
	Details     string         `json:"details"`
	Help        []string       `json:"help"`
}

// FullResult is the expanded projection.
type FullResult struct {
	Status      string   `json:"status"`
	Environment string   `json:"environment,omitempty"`
	Site        string   `json:"site,omitempty"`
	Project     Project  `json:"project"`
	RequestID   string   `json:"tableau_request_id,omitempty"`
	Help        []string `json:"help"`
}

// CompactOutput returns exact identity fields.
func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Project: CompactProject{LUID: o.Project.LUID, Name: o.Project.Name, Path: o.Project.Path, ParentLUID: o.Project.ParentLUID}, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded lifecycle details.
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Project: o.Project, RequestID: o.RequestID, Help: o.Help}
}
