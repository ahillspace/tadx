package datasource

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
)

const inspectDetailLimit = 50

// InspectInput selects one exact published datasource.
type InspectInput struct {
	Environment string
	Site        string
	Selector    identity.Selector
	Cache       bool
}

// SetSelector records one authoritative or exact selector.
func (i *InspectInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// SetSelectorWithProjectLUID records an authoritative or exact selector using a project LUID.
func (i *InspectInput) SetSelectorWithProjectLUID(luid, name, projectPath, projectLUID string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath, ProjectLUID: identity.LUID(projectLUID)}
}

// InspectOutput retains complete details before projection.
type InspectOutput struct {
	Status      string
	Environment string
	Site        string
	Datasource  InspectDatasource
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// InspectCompactDatasource is the exact identity and lifecycle summary.
type InspectCompactDatasource struct {
	Upstream    *InspectCompactUpstream `json:"upstream,omitempty"`
	LUID        string                  `json:"luid"`
	Name        string                  `json:"name"`
	ProjectLUID string                  `json:"project_luid"`
	ProjectPath string                  `json:"project_path"`
	Type        string                  `json:"type,omitempty"`
	ContentURL  string                  `json:"content_url,omitempty"`
	UpdatedAt   string                  `json:"updated_at,omitempty"`
}

// InspectCompactResult is the default projection.
type InspectCompactResult struct {
	Status      string                   `json:"status"`
	Environment string                   `json:"environment,omitempty"`
	Site        string                   `json:"site,omitempty"`
	Datasource  InspectCompactDatasource `json:"datasource"`
	Details     string                   `json:"details"`
	Help        []string                 `json:"help"`
	Source      *readsource.Metadata     `json:"source,omitempty"`
}

// InspectFullResult is the bounded expanded projection.
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Datasource  InspectDatasource    `json:"datasource"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns exact identity fields.
func (o InspectOutput) CompactOutput() any {
	item := o.Datasource
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Datasource: InspectCompactDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, Type: item.Type, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt, Upstream: inspectCompactUpstream(item.Upstream)}, Details: "--full", Help: o.Help, Source: o.Source}
}

type Upstream = value.DatasourceUpstream
type InspectCompactUpstreamTable struct {
	value.MetadataIdentity
	Database value.MetadataIdentity `json:"database"`
}
type InspectCompactUpstream struct {
	Status    string                        `json:"status"`
	Databases []value.MetadataIdentity      `json:"databases"`
	Tables    []InspectCompactUpstreamTable `json:"tables"`
	Complete  bool                          `json:"complete"`
	Help      string                        `json:"help,omitempty"`
}

func inspectCompactUpstream(input *Upstream) *InspectCompactUpstream {
	if input == nil {
		return nil
	}
	out := &InspectCompactUpstream{Status: input.Status, Complete: input.Complete, Help: input.Help, Databases: []value.MetadataIdentity{}, Tables: []InspectCompactUpstreamTable{}}
	for _, db := range input.Databases {
		out.Databases = append(out.Databases, db.MetadataIdentity)
	}
	for _, table := range input.Tables {
		out.Tables = append(out.Tables, InspectCompactUpstreamTable{MetadataIdentity: table.MetadataIdentity, Database: table.Database})
	}
	return out
}

// FullOutput returns bounded REST metadata and tags.
func (o InspectOutput) FullOutput() any {
	item := o.Datasource
	item.Tags = append([]string(nil), item.Tags...)
	if len(item.Tags) > inspectDetailLimit {
		item.TagsOmitted = len(item.Tags) - inspectDetailLimit
		item.Tags = item.Tags[:inspectDetailLimit]
	}
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Datasource: item, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type InspectDatasource struct {
	Upstream            *Upstream `json:"upstream,omitempty"`
	LUID                string    `json:"luid"`
	Name                string    `json:"name"`
	ProjectLUID         string    `json:"project_luid"`
	ProjectPath         string    `json:"project_path"`
	Type                string    `json:"type,omitempty"`
	ContentURL          string    `json:"content_url,omitempty"`
	Description         string    `json:"description,omitempty"`
	OwnerLUID           string    `json:"owner_luid,omitempty"`
	CreatedAt           string    `json:"created_at,omitempty"`
	UpdatedAt           string    `json:"updated_at,omitempty"`
	Size                *int64    `json:"size,omitempty"`
	EncryptExtracts     *bool     `json:"encrypt_extracts,omitempty"`
	HasExtracts         *bool     `json:"has_extracts,omitempty"`
	IsCertified         *bool     `json:"is_certified,omitempty"`
	CertificationNote   string    `json:"certification_note,omitempty"`
	UseRemoteQueryAgent *bool     `json:"use_remote_query_agent,omitempty"`
	WebpageURL          string    `json:"webpage_url,omitempty"`
	Tags                []string  `json:"tags,omitempty"`
	AskDataEnablement   string    `json:"ask_data_enablement,omitempty"`
	TagsOmitted         int       `json:"tags_omitted,omitempty"`
	RequestID           string    `json:"-"`
}
