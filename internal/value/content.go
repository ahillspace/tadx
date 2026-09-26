// Package value contains dependency-free, shared identity and normalized metadata values.
// Action inputs, outputs, behavior, and provider payloads remain in their owning packages.
package value

// ContentIdentity identifies exact REST content and its containing project.
type ContentIdentity struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
}

// OwnedContentIdentity adds the owner required by move and update actions.
type OwnedContentIdentity struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid"`
	ProjectPath string `json:"project_path"`
	OwnerLUID   string `json:"owner_luid"`
}

// ProjectIdentity identifies one authoritative project destination.
type ProjectIdentity struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Workbook is the normalized workbook record shared by lifecycle operations and
// resource adapters. Each operation owns its separate external projection.
type Workbook struct {
	LUID        string   `json:"luid"`
	Name        string   `json:"name"`
	ProjectLUID string   `json:"project_luid"`
	ProjectPath string   `json:"project_path"`
	ContentURL  string   `json:"content_url,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Description string   `json:"description,omitempty"`
	OwnerLUID   string   `json:"owner_luid,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	RequestID   string   `json:"-"`
}

// Datasource is the normalized lifecycle record. Operations separately project
// public results; Upstream is optional independently observed metadata.
type Datasource struct {
	Upstream            *DatasourceUpstream `json:"upstream,omitempty"`
	LUID                string              `json:"luid"`
	Name                string              `json:"name"`
	ProjectLUID         string              `json:"project_luid"`
	ProjectName         string              `json:"project_name,omitempty"`
	ProjectPath         string              `json:"project_path"`
	Type                string              `json:"type,omitempty"`
	ContentURL          string              `json:"content_url,omitempty"`
	Description         string              `json:"description,omitempty"`
	OwnerLUID           string              `json:"owner_luid,omitempty"`
	CreatedAt           string              `json:"created_at,omitempty"`
	UpdatedAt           string              `json:"updated_at,omitempty"`
	Size                *int64              `json:"size,omitempty"`
	EncryptExtracts     *bool               `json:"encrypt_extracts,omitempty"`
	HasExtracts         *bool               `json:"has_extracts,omitempty"`
	IsCertified         *bool               `json:"is_certified,omitempty"`
	CertificationNote   string              `json:"certification_note,omitempty"`
	UseRemoteQueryAgent *bool               `json:"use_remote_query_agent,omitempty"`
	WebpageURL          string              `json:"webpage_url,omitempty"`
	Tags                []string            `json:"tags,omitempty"`
	AskDataEnablement   string              `json:"ask_data_enablement,omitempty"`
	RequestID           string              `json:"-"`
}

// DatasourceUpstream preserves one separately observed upstream metadata result.
type DatasourceUpstream struct {
	Status     string             `json:"status"`
	Databases  []MetadataDatabase `json:"databases"`
	Tables     []MetadataTable    `json:"tables"`
	Complete   bool               `json:"complete"`
	ObservedAt string             `json:"observed_at,omitempty"`
	Help       string             `json:"help,omitempty"`
}

// Flow is the normalized record shared by flow lifecycle operations.
// Each operation owns its external projection.
type Flow struct {
	LUID        string           `json:"luid"`
	Name        string           `json:"name"`
	ProjectLUID string           `json:"project_luid"`
	ProjectName string           `json:"project_name,omitempty"`
	ProjectPath string           `json:"project_path,omitempty"`
	FileType    string           `json:"file_type,omitempty"`
	UpdatedAt   string           `json:"updated_at,omitempty"`
	Description string           `json:"description,omitempty"`
	OwnerLUID   string           `json:"owner_luid,omitempty"`
	CreatedAt   string           `json:"created_at,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Parameters  []FlowParameter  `json:"parameters,omitempty"`
	OutputSteps []FlowOutputStep `json:"output_steps,omitempty"`
	RequestID   string           `json:"-"`
}

type FlowParameter struct {
	LUID        string `json:"luid,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Value       string `json:"value,omitempty"`
	Required    *bool  `json:"required,omitempty"`
}

type FlowOutputStep struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
}
