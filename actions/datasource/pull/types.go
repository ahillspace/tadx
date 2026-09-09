package pull

import (
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

// Input selects one remote datasource and one logical workspace.
type Input struct {
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName                                        string
	Environment, Site, ServerOrigin, SiteLUID, Workspace string
	Selector                                             identity.Selector
	Overwrite                                            bool
}

// SetSelector records one exact datasource selector.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// Datasource is one authoritative remote datasource identity.
type Datasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
}

// Download contains unchanged native datasource bytes.
type Download struct {
	Filename         string
	Content          []byte
	TableauRequestID string
}

// LineageRequest selects one bounded Metadata API capture.
type LineageRequest struct {
	Kind, RESTLUID, Direction string
	Depth                     int
}

type LineageNode struct{ MetadataID, Kind, RESTLUID, Name string }
type LineageEdge struct{ FromMetadataID, ToMetadataID, Relationship string }

// Lineage contains one bounded factual graph.
type Lineage struct {
	Complete  bool
	Direction string
	Depth     int
	Nodes     []LineageNode
	Edges     []LineageEdge
	Warnings  []string
}

// Artifact is the complete datasource persistence request.
type Artifact struct {
	Workspace, Filename, Name, TableauID, Environment, Site, ServerOrigin, SiteLUID, ProjectName, ProjectID string
	Content                                                                                                 []byte
	Lineage                                                                                                 Lineage
	Overwrite                                                                                               bool
}

// ArtifactResult identifies one materialized datasource artifact.
type ArtifactResult struct {
	Path                 string   `json:"path"`
	CanonicalPath        string   `json:"canonical_path,omitempty"`
	BaselineFingerprint  string   `json:"baseline_fingerprint,omitempty"`
	LineagePath          string   `json:"lineage_path,omitempty"`
	LineageStatus        string   `json:"lineage_status"`
	CompositionStatus    string   `json:"composition_status"`
	ParentDataSourceURLs []string `json:"parent_datasource_urls,omitempty"`
	NodeCount            int      `json:"node_count"`
	EdgeCount            int      `json:"edge_count"`
	CountsKnown          bool     `json:"-"`
	Warnings             []string `json:"-"`
}

// Output retains bounded pull details before projection.
type Output struct {
	Workspace  string `json:"workspace"`
	Status     string
	Datasource Datasource
	Artifact   ArtifactResult
	Warnings   []string
	RequestID  string
	Help       []string
}

type CompactArtifact struct {
	Workspace         string `json:"workspace"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	SourceLUID        string `json:"source_luid"`
	Path              string `json:"-"`
	CompositionStatus string `json:"composition_status"`
}

type CompactResult struct {
	Status          string          `json:"status"`
	Datasource      Datasource      `json:"datasource"`
	Artifact        CompactArtifact `json:"artifact"`
	Warnings        []string        `json:"warnings,omitempty"`
	WarningsOmitted int             `json:"warnings_omitted,omitempty"`
	Details         string          `json:"details"`
	Help            []string        `json:"help"`
}

type FullArtifact struct {
	Workspace            string   `json:"workspace"`
	Kind                 string   `json:"kind"`
	Name                 string   `json:"name"`
	SourceLUID           string   `json:"source_luid"`
	Path                 string   `json:"path"`
	CanonicalPath        string   `json:"canonical_path,omitempty"`
	BaselineFingerprint  string   `json:"baseline_fingerprint,omitempty"`
	LineagePath          string   `json:"lineage_path,omitempty"`
	LineageStatus        string   `json:"lineage_status"`
	CompositionStatus    string   `json:"composition_status"`
	ParentDataSourceURLs []string `json:"parent_datasource_urls,omitempty"`
	NodeCount            *int     `json:"node_count,omitempty"`
	EdgeCount            *int     `json:"edge_count,omitempty"`
}

type FullResult struct {
	Status          string       `json:"status"`
	Datasource      Datasource   `json:"datasource"`
	Artifact        FullArtifact `json:"artifact"`
	Warnings        []string     `json:"warnings,omitempty"`
	WarningsOmitted int          `json:"warnings_omitted,omitempty"`
	RequestID       string       `json:"tableau_request_id,omitempty"`
	Help            []string     `json:"help"`
}

func (o Output) CompactOutput() any {
	warnings, omitted := boundedWarnings(o.Warnings)
	return CompactResult{Status: o.Status, Datasource: o.Datasource, Artifact: CompactArtifact{Workspace: o.Workspace, Kind: "datasource", Name: o.Datasource.Name, SourceLUID: o.Datasource.LUID, Path: o.Artifact.Path, CompositionStatus: o.Artifact.CompositionStatus}, Warnings: warnings, WarningsOmitted: omitted, Details: "--full", Help: o.Help}
}

func (o Output) FullOutput() any {
	warnings, omitted := boundedWarnings(o.Warnings)
	artifact := FullArtifact{Workspace: o.Workspace, Kind: "datasource", Name: o.Datasource.Name, SourceLUID: o.Datasource.LUID, Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath, BaselineFingerprint: o.Artifact.BaselineFingerprint, LineagePath: o.Artifact.LineagePath, LineageStatus: o.Artifact.LineageStatus, CompositionStatus: o.Artifact.CompositionStatus, ParentDataSourceURLs: append([]string(nil), o.Artifact.ParentDataSourceURLs...)}
	if o.Artifact.CountsKnown {
		nodes, edges := o.Artifact.NodeCount, o.Artifact.EdgeCount
		artifact.NodeCount, artifact.EdgeCount = &nodes, &edges
	}
	return FullResult{Status: o.Status, Datasource: o.Datasource, Artifact: artifact, Warnings: warnings, WarningsOmitted: omitted, RequestID: o.RequestID, Help: o.Help}
}

const warningLimit = 20

func boundedWarnings(values []string) ([]string, int) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, min(len(values), warningLimit))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		if len(result) < warningLimit {
			result = append(result, value)
		}
	}
	return result, max(0, len(seen)-len(result))
}
