package pull

import "github.com/ahillspace/tadx/internal/identity"

const (
	maxFullPublishedDatasourceDetails = 50
	maxOutputWarnings                 = 20
	maxLineageNodes                   = 500
	maxLineageEdges                   = 1000
	maxLineageWarningBytes            = 512
)

// Input selects one remote workbook and explicit existing workspace.
type Input struct {
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName  string
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
	IncludePDS     bool
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

// LineageRequest selects the automatic bounded workbook lineage capture.
type LineageRequest struct {
	RESTLUID  string
	Direction string
	Depth     int
}

// LineageNode preserves distinct Metadata and REST identities.
type LineageNode struct {
	MetadataID string `json:"metadata_id"`
	Kind       string `json:"kind"`
	RESTLUID   string `json:"rest_luid,omitempty"`
	Name       string `json:"name,omitempty"`
}

// LineageEdge is one factual directed relationship.
type LineageEdge struct {
	FromMetadataID string `json:"from_metadata_id"`
	ToMetadataID   string `json:"to_metadata_id"`
	Relationship   string `json:"relationship"`
}

// LineageCapture is one bounded best-effort workbook graph.
type LineageCapture struct {
	RootMetadataID string        `json:"root_metadata_id,omitempty"`
	Complete       bool          `json:"complete"`
	Direction      string        `json:"direction"`
	Depth          int           `json:"depth"`
	Nodes          []LineageNode `json:"nodes"`
	Edges          []LineageEdge `json:"edges"`
	Warnings       []string      `json:"warnings,omitempty"`
}

// PublishedDatasource is one direct dependency discovered through authoritative metadata.
type PublishedDatasource struct {
	LUID string `json:"luid"`
	Name string `json:"name,omitempty"`
}

// PublishedDatasourceRef is the provenance recorded on the workbook artifact.
type PublishedDatasourceRef struct {
	LUID                string `json:"luid"`
	Name                string `json:"name,omitempty"`
	SourceSite          string `json:"source_site,omitempty"`
	LocalArtifactPath   string `json:"local_artifact_path,omitempty"`
	CanonicalPath       string `json:"canonical_path,omitempty"`
	BaselineFingerprint string `json:"baseline_fingerprint,omitempty"`
}

// DatasourceDownload preserves one direct published datasource dependency.
type DatasourceDownload struct {
	LUID             string
	Name             string
	ProjectLUID      string
	ProjectPath      string
	Filename         string
	Content          []byte
	TableauRequestID string
}

// DatasourceArtifact is one dependency persistence request.
type DatasourceArtifact struct {
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

// DependencyArtifactResult is one materialized datasource sibling.
type DependencyArtifactResult struct {
	LUID                string   `json:"luid"`
	Name                string   `json:"name,omitempty"`
	Path                string   `json:"path"`
	CanonicalPath       string   `json:"canonical_path,omitempty"`
	BaselineFingerprint string   `json:"baseline_fingerprint"`
	Warnings            []string `json:"-"`
}

// Artifact is the complete local write request.
type Artifact struct {
	Workspace            string
	Filename             string
	Content              []byte
	Name                 string
	TableauID            string
	Environment          string
	Site                 string
	ServerOrigin         string
	SiteLUID             string
	ProjectName          string
	ProjectID            string
	Portability          string
	PublishedDatasources []PublishedDatasourceRef
	DependenciesAcquired bool
	Lineage              LineageCapture
	LineageCountsKnown   bool
	Overwrite            bool
	TableauRequestID     string
}

// ArtifactResult is the local manager result.
type ArtifactResult struct {
	Path                 string                     `json:"path"`
	CanonicalPath        string                     `json:"canonical_path,omitempty"`
	BaselineFingerprint  string                     `json:"baseline_fingerprint"`
	Portability          string                     `json:"portability,omitempty"`
	PublishedDatasources []PublishedDatasourceRef   `json:"published_datasources,omitempty"`
	DependenciesAcquired bool                       `json:"dependencies_acquired"`
	LineagePath          string                     `json:"lineage_path,omitempty"`
	LineageStatus        string                     `json:"lineage_status,omitempty"`
	LineageNodeCount     *int                       `json:"lineage_node_count,omitempty"`
	LineageEdgeCount     *int                       `json:"lineage_edge_count,omitempty"`
	Dependencies         []DependencyArtifactResult `json:"-"`
	Warnings             []string                   `json:"-"`
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

// CompactWorkbook is the authoritative identity needed after a pull.
type CompactWorkbook struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
}

// CompactArtifact is the bounded actionable artifact summary.
type CompactArtifact struct {
	Path                     string `json:"path"`
	Portability              string `json:"portability"`
	PublishedDatasourceCount *int   `json:"published_datasource_count,omitempty"`
	DependenciesAcquired     bool   `json:"dependencies_acquired"`
}

// CompactResult is the standard token-bounded workbook.pull response.
type CompactResult struct {
	Status          string          `json:"status"`
	Workbook        CompactWorkbook `json:"workbook"`
	Artifact        CompactArtifact `json:"artifact"`
	Warnings        []string        `json:"warnings,omitempty"`
	WarningsOmitted int             `json:"warnings_omitted,omitempty"`
	Details         string          `json:"details"`
	Help            []string        `json:"help"`
}

// FullArtifact is the bounded expanded artifact view.
type FullArtifact struct {
	Path                        string                   `json:"path"`
	CanonicalPath               string                   `json:"canonical_path,omitempty"`
	BaselineFingerprint         string                   `json:"baseline_fingerprint,omitempty"`
	Portability                 string                   `json:"portability"`
	PublishedDatasourceCount    *int                     `json:"published_datasource_count,omitempty"`
	PublishedDatasources        []PublishedDatasourceRef `json:"published_datasources,omitempty"`
	PublishedDatasourcesOmitted int                      `json:"published_datasources_omitted,omitempty"`
	DependenciesAcquired        bool                     `json:"dependencies_acquired"`
	LineagePath                 string                   `json:"lineage_path,omitempty"`
	LineageStatus               string                   `json:"lineage_status,omitempty"`
	LineageNodeCount            *int                     `json:"lineage_node_count,omitempty"`
	LineageEdgeCount            *int                     `json:"lineage_edge_count,omitempty"`
}

// FullResult is the bounded expanded workbook.pull response.
type FullResult struct {
	Status          string       `json:"status"`
	Workbook        Workbook     `json:"workbook"`
	Artifact        FullArtifact `json:"artifact"`
	Warnings        []string     `json:"warnings,omitempty"`
	WarningsOmitted int          `json:"warnings_omitted,omitempty"`
	RequestID       string       `json:"tableau_request_id,omitempty"`
	Help            []string     `json:"help"`
}

// CompactOutput returns the standard response without provenance diagnostics.
func (o Output) CompactOutput() any {
	publishedDatasourceCount := knownPublishedDatasourceCount(o.Artifact)
	warnings, warningsOmitted := boundedWarnings(o.Warnings)
	return CompactResult{
		Status: o.Status,
		Workbook: CompactWorkbook{
			LUID: o.Workbook.LUID, Name: o.Workbook.Name, ProjectPath: o.Workbook.ProjectPath,
		},
		Artifact: CompactArtifact{
			Path: o.Artifact.Path, Portability: o.Artifact.Portability,
			PublishedDatasourceCount: publishedDatasourceCount,
			DependenciesAcquired:     o.Artifact.DependenciesAcquired,
		},
		Warnings:        warnings,
		WarningsOmitted: warningsOmitted,
		Details:         "--full",
		Help:            o.Help,
	}
}

// FullOutput returns bounded provenance and diagnostics for the same pull.
func (o Output) FullOutput() any {
	publishedDatasources := o.Artifact.PublishedDatasources
	publishedDatasourcesOmitted := 0
	if len(publishedDatasources) > maxFullPublishedDatasourceDetails {
		publishedDatasourcesOmitted = len(publishedDatasources) - maxFullPublishedDatasourceDetails
		publishedDatasources = publishedDatasources[:maxFullPublishedDatasourceDetails]
	}
	warnings, warningsOmitted := boundedWarnings(o.Warnings)
	return FullResult{
		Status:   o.Status,
		Workbook: o.Workbook,
		Artifact: FullArtifact{
			Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath,
			BaselineFingerprint: o.Artifact.BaselineFingerprint, Portability: o.Artifact.Portability,
			PublishedDatasourceCount: knownPublishedDatasourceCount(o.Artifact),
			PublishedDatasources:     publishedDatasources, PublishedDatasourcesOmitted: publishedDatasourcesOmitted,
			DependenciesAcquired: o.Artifact.DependenciesAcquired,
			LineagePath:          o.Artifact.LineagePath, LineageStatus: o.Artifact.LineageStatus,
			LineageNodeCount: o.Artifact.LineageNodeCount, LineageEdgeCount: o.Artifact.LineageEdgeCount,
		},
		Warnings: warnings, WarningsOmitted: warningsOmitted,
		RequestID: o.RequestID, Help: o.Help,
	}
}

func knownPublishedDatasourceCount(artifact ArtifactResult) *int {
	if artifact.Portability == "unknown" {
		return nil
	}
	count := len(artifact.PublishedDatasources)
	return &count
}

func boundedWarnings(input []string) ([]string, int) {
	unique := make([]string, 0, len(input))
	seen := make(map[string]bool, len(input))
	for _, warning := range input {
		if warning == "" || seen[warning] {
			continue
		}
		seen[warning] = true
		unique = append(unique, warning)
	}
	if len(unique) <= maxOutputWarnings {
		return unique, 0
	}
	return unique[:maxOutputWarnings], len(unique) - maxOutputWarnings
}
