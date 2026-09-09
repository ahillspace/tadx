package pull

// Input selects one Pulse definition and a managed workspace.
type Input struct {
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
	LUID           string
	Name           string
	DatasourceLUID string
	Configuration  []byte
	RequestID      string
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
	Overwrite      bool
}

// ArtifactResult identifies one managed definition artifact.
type ArtifactResult struct {
	Path                string `json:"path"`
	CanonicalPath       string `json:"canonical_path,omitempty"`
	BaselineFingerprint string `json:"baseline_fingerprint,omitempty"`
}

// Output retains complete details before projection.
type Output struct {
	Status     string
	Definition Definition
	Artifact   ArtifactResult
	RequestID  string
	Help       []string
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
	Status     string            `json:"status"`
	Definition CompactDefinition `json:"definition"`
	Artifact   CompactArtifact   `json:"artifact"`
	Details    string            `json:"details"`
	Help       []string          `json:"help"`
}

// FullResult is the expanded projection.
type FullResult struct {
	Status     string            `json:"status"`
	Definition CompactDefinition `json:"definition"`
	Artifact   ArtifactResult    `json:"artifact"`
	RequestID  string            `json:"tableau_request_id,omitempty"`
	Help       []string          `json:"help"`
}

// CompactOutput returns the managed path and exact definition identity.
func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Definition: CompactDefinition{LUID: o.Definition.LUID, Name: o.Definition.Name}, Artifact: CompactArtifact{Path: o.Artifact.Path}, Details: "--full", Help: o.Help}
}

// FullOutput returns artifact provenance without embedding the full resource document.
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Definition: CompactDefinition{LUID: o.Definition.LUID, Name: o.Definition.Name}, Artifact: o.Artifact, RequestID: o.RequestID, Help: o.Help}
}
