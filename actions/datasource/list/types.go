package list

import "github.com/ahillspace/tadx/internal/readsource"

const fullTagsPerDatasourceLimit = 50

// Input selects one published datasource page.
type Input struct {
	Environment   string
	Site          string
	Cursor        string
	Name          string
	OwnerName     string
	ProjectName   string
	Type          string
	Tag           string
	UpdatedAfter  string
	UpdatedBefore string
	Limit         int
	Catalog       bool
}

// PageRequest is the action-owned bounded read request.
type PageRequest struct {
	PageNumber     int
	PageSize       int
	Name           string
	OwnerName      string
	ProjectName    string
	Type           string
	Tag            string
	UpdatedAfter   string
	UpdatedBefore  string
	SnapshotCursor string
}

// Datasource is one complete lifecycle projection.
type Datasource struct {
	LUID                string   `json:"luid"`
	Name                string   `json:"name"`
	ProjectLUID         string   `json:"project_luid"`
	ProjectName         string   `json:"project_name,omitempty"`
	ProjectPath         string   `json:"project_path,omitempty"`
	Type                string   `json:"type,omitempty"`
	ContentURL          string   `json:"content_url,omitempty"`
	Description         string   `json:"description,omitempty"`
	OwnerLUID           string   `json:"owner_luid,omitempty"`
	CreatedAt           string   `json:"created_at,omitempty"`
	UpdatedAt           string   `json:"updated_at,omitempty"`
	Size                *int64   `json:"size,omitempty"`
	EncryptExtracts     *bool    `json:"encrypt_extracts,omitempty"`
	HasExtracts         *bool    `json:"has_extracts,omitempty"`
	IsCertified         *bool    `json:"is_certified,omitempty"`
	CertificationNote   string   `json:"certification_note,omitempty"`
	UseRemoteQueryAgent *bool    `json:"use_remote_query_agent,omitempty"`
	WebpageURL          string   `json:"webpage_url,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	AskDataEnablement   string   `json:"ask_data_enablement,omitempty"`
	TagsOmitted         int      `json:"tags_omitted,omitempty"`
}

// Page is one complete reader page.
type Page struct {
	Number               int
	Size                 int
	Total                int
	Datasources          []Datasource
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}

// OutputPage is bounded continuation metadata.
type OutputPage struct {
	Returned   int    `json:"returned"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Output retains the complete current page before projection.
type Output struct {
	Status      string
	Environment string
	Site        string
	Page        OutputPage
	Datasources []Datasource
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// CompactDatasource contains the identity and lifecycle fields used for the next decision.
type CompactDatasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectName string `json:"project_name,omitempty"`
	Type        string `json:"type,omitempty"`
	ContentURL  string `json:"content_url,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// CompactResult is the default bounded projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Datasources []CompactDatasource  `json:"datasources"`
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
	Datasources []Datasource         `json:"datasources"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns explicit compact fields.
func (o Output) CompactOutput() any {
	items := make([]CompactDatasource, len(o.Datasources))
	for index, item := range o.Datasources {
		items[index] = CompactDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, Type: item.Type, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Datasources: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns the same page with bounded lifecycle metadata and tags.
func (o Output) FullOutput() any {
	items := append([]Datasource(nil), o.Datasources...)
	for index := range items {
		items[index].Tags = append([]string(nil), items[index].Tags...)
		if len(items[index].Tags) > fullTagsPerDatasourceLimit {
			items[index].TagsOmitted = len(items[index].Tags) - fullTagsPerDatasourceLimit
			items[index].Tags = items[index].Tags[:fullTagsPerDatasourceLimit]
		}
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Datasources: items, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
