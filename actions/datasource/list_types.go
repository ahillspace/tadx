package datasource

import (
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

const listFullTagsPerDatasourceLimit = 50

// ListInput selects one published datasource page.
type ListInput struct {
	All           bool
	Environment   string
	Site          string
	Cursor        string
	Name          string
	OwnerName     string
	ProjectLUID   string
	ProjectName   string
	Type          string
	Tag           string
	UpdatedAfter  string
	UpdatedBefore string
	Limit         int
	Cache         bool
}

// ListPageRequest is the action-owned bounded read request.
type ListPageRequest struct {
	PageNumber     int
	PageSize       int
	Name           string
	OwnerName      string
	ProjectLUID    string
	ProjectName    string
	Type           string
	Tag            string
	UpdatedAfter   string
	UpdatedBefore  string
	SnapshotCursor string
}

// ListPage is one complete reader page.
type ListPage struct {
	Number               int
	Size                 int
	Total                int
	Datasources          []Record
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}

// ListOutputPage is bounded continuation metadata.
type ListOutputPage = output.Page

// ListOutput retains the complete current page before projection.
type ListOutput struct {
	Status      string
	Environment string
	Site        string
	Page        ListOutputPage
	Datasources []Record
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// ListCompactDatasource contains the identity and lifecycle fields used for the next decision.
type ListCompactDatasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectName string `json:"project_name"`
	Type        string `json:"type"`
	ContentURL  string `json:"content_url"`
	UpdatedAt   string `json:"updated_at"`
}

// ListCompactResult is the default bounded projection.
type ListCompactResult struct {
	Status      string                  `json:"status"`
	Environment string                  `json:"environment,omitempty"`
	Site        string                  `json:"site,omitempty"`
	Page        ListOutputPage          `json:"page"`
	Datasources []ListCompactDatasource `json:"datasources"`
	Details     string                  `json:"details"`
	Help        []string                `json:"help"`
	Source      *readsource.Metadata    `json:"source,omitempty"`
}

// ListFullResult is the bounded expanded current page.
type ListFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Datasources []listDatasource     `json:"datasources"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns explicit compact fields.
func (o ListOutput) CompactOutput() any {
	items := make([]ListCompactDatasource, len(o.Datasources))
	for index, item := range o.Datasources {
		items[index] = ListCompactDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, Type: item.Type, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt}
	}
	return ListCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Datasources: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns the same page with bounded lifecycle metadata and tags.
func (o ListOutput) FullOutput() any {
	items := make([]listDatasource, len(o.Datasources))
	for index, item := range o.Datasources {
		items[index] = listRecord(item)
		if len(items[index].Tags) > listFullTagsPerDatasourceLimit {
			items[index].TagsOmitted = len(items[index].Tags) - listFullTagsPerDatasourceLimit
			items[index].Tags = items[index].Tags[:listFullTagsPerDatasourceLimit]
		}
	}
	return ListFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Datasources: items, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type listDatasource struct {
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
