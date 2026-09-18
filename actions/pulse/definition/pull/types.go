package pull

import "github.com/ahillspace/tadx/internal/value"

// Input selects one Pulse definition and a managed workspace.
type Input struct {
	Preview       bool
	Environment   string
	Site          string
	ServerOrigin  string
	SiteLUID      string
	Workspace     string
	WorkspaceName string
	LUID          string
	Overwrite     bool
}

// Definition is the canonical remote definition document.
type Definition struct {
	LUID            string
	Name            string
	DatasourceLUID  string
	Configuration   []byte
	RequestID       string
	Metrics         []Metric
	MetricsComplete bool
}

type Metric struct {
	LUID           string
	DefinitionLUID string
	IsDefault      bool
	Specification  []byte
}

// Artifact is the action-owned materialization request.
type Artifact struct {
	Workspace      string
	DefinitionLUID string
	Name           string
	DatasourceLUID string
	Environment    string
	Site           string
	ServerOrigin   string
	SiteLUID       string
	Configuration  []byte
	Metrics        []Metric
	Overwrite      bool
}

// ArtifactResult identifies one managed definition artifact.
type ArtifactResult struct {
	Path                string `json:"path"`
	CanonicalPath       string `json:"canonical_path,omitempty"`
	BaselineFingerprint string `json:"baseline_fingerprint,omitempty"`
}

// Provenance identifies the source environment without embedding credentials.
type Provenance struct {
	Environment    string `json:"environment,omitempty"`
	Site           string `json:"site,omitempty"`
	ServerOrigin   string `json:"server_origin,omitempty"`
	SiteLUID       string `json:"site_luid,omitempty"`
	Workspace      string `json:"workspace,omitempty"`
	DatasourceLUID string `json:"datasource_luid,omitempty"`
}

// Output retains complete details before projection.
type Output struct {
	Preview     *value.AcquisitionPlan
	Status      string
	Definition  Definition
	Artifact    ArtifactResult
	Provenance  Provenance
	RequestID   string
	MetricCount int
	Help        []string
}

// CompactDefinition identifies the pulled definition.
type CompactDefinition struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
}

// CompactArtifact identifies the managed artifact directory.
type CompactArtifact struct {
	Path string `json:"path"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Status      string            `json:"status"`
	Definition  CompactDefinition `json:"definition"`
	Artifact    CompactArtifact   `json:"artifact"`
	MetricCount int               `json:"metric_count"`
	Provenance  Provenance        `json:"provenance"`
	Details     string            `json:"details"`
	Help        []string          `json:"help"`
}

// FullResult is the expanded projection.
type FullResult struct {
	Status      string            `json:"status"`
	Definition  CompactDefinition `json:"definition"`
	Artifact    ArtifactResult    `json:"artifact"`
	MetricCount int               `json:"metric_count"`
	Provenance  Provenance        `json:"provenance"`
	RequestID   string            `json:"tableau_request_id,omitempty"`
	Help        []string          `json:"help"`
}

// CompactOutput returns the managed path and exact definition identity.
func (o Output) CompactOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	return CompactResult{Status: o.Status, Definition: CompactDefinition{LUID: o.Definition.LUID, Name: o.Definition.Name}, Artifact: CompactArtifact{Path: o.Artifact.Path}, MetricCount: o.MetricCount, Provenance: o.Provenance, Details: "--full", Help: o.Help}
}

// FullOutput returns artifact provenance without embedding the full resource document.
func (o Output) FullOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	return FullResult{Status: o.Status, Definition: CompactDefinition{LUID: o.Definition.LUID, Name: o.Definition.Name}, Artifact: o.Artifact, MetricCount: o.MetricCount, Provenance: o.Provenance, RequestID: o.RequestID, Help: o.Help}
}
