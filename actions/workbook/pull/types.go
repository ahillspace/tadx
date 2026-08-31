package pull

import "github.com/ahillspace/tadx/internal/identity"

// Input selects one remote workbook and explicit existing workspace.
type Input struct {
	Environment    string
	Site           string
	ServerOrigin   string
	SiteLUID       string
	Workspace      string
	Selector       identity.Selector
	LUID           string
	Name           string
	ProjectPath    string
	IncludeExtract *bool
	Overwrite      bool
}

// Workbook is the exact authoritative remote workbook.
type Workbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
}

// Download preserves native bytes before local storage.
type Download struct {
	Filename         string
	Content          []byte
	TableauRequestID string
}

// Artifact is the complete local write request.
type Artifact struct {
	Workspace        string
	Filename         string
	Content          []byte
	Name             string
	TableauID        string
	Environment      string
	Site             string
	ServerOrigin     string
	SiteLUID         string
	ProjectName      string
	ProjectID        string
	Overwrite        bool
	TableauRequestID string
}

// ArtifactResult is the local manager result.
type ArtifactResult struct {
	Path                string   `json:"path"`
	CanonicalPath       string   `json:"canonical_path,omitempty"`
	BaselineFingerprint string   `json:"baseline_fingerprint"`
	Warnings            []string `json:"warnings,omitempty"`
}

// Output is the stable pull result.
type Output struct {
	Status    string         `json:"status"`
	Workbook  Workbook       `json:"workbook"`
	Artifact  ArtifactResult `json:"artifact"`
	Warnings  []string       `json:"warnings,omitempty"`
	RequestID string         `json:"tableau_request_id,omitempty"`
	Help      []string       `json:"help"`
}
