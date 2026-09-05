package inspect

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
)

const detailLimit = 50

// Input selects one exact published datasource.
type Input struct {
	Environment string
	Site        string
	Selector    identity.Selector
	Catalog     bool
}

// SetSelector records one authoritative or exact selector.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// Datasource is one complete exact published datasource projection.
type Datasource struct {
	LUID                string   `json:"luid"`
	Name                string   `json:"name"`
	ProjectLUID         string   `json:"project_luid"`
	ProjectPath         string   `json:"project_path"`
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
	RequestID           string   `json:"-"`
}

// Output retains complete details before projection.
type Output struct {
	Status      string
	Environment string
	Site        string
	Datasource  Datasource
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// CompactDatasource is the exact identity and lifecycle summary.
type CompactDatasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	Type        string `json:"type,omitempty"`
	ContentURL  string `json:"content_url,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Datasource  CompactDatasource    `json:"datasource"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the bounded expanded projection.
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Datasource  Datasource           `json:"datasource"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns exact identity fields.
func (o Output) CompactOutput() any {
	item := o.Datasource
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Datasource: CompactDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, Type: item.Type, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt}, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns bounded REST metadata and tags.
func (o Output) FullOutput() any {
	item := o.Datasource
	item.Tags = append([]string(nil), item.Tags...)
	if len(item.Tags) > detailLimit {
		item.TagsOmitted = len(item.Tags) - detailLimit
		item.Tags = item.Tags[:detailLimit]
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Datasource: item, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
