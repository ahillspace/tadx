package datasource

import "github.com/ahillspace/tadx/internal/value"

import (
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

// PullInput selects one remote datasource and one logical workspace.
type PullInput struct {
	Preview bool
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName                                        string
	Environment, Site, ServerOrigin, SiteLUID, Workspace string
	Selector                                             identity.Selector
	Overwrite                                            bool
}

// SetSelector records one exact datasource selector.
func (i *PullInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
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

type LineageNode = value.LineageNode
type LineageEdge = value.LineageEdge

// Lineage contains one bounded factual graph.
type Lineage struct {
	Complete  bool
	Direction string
	Depth     int
	Failure   *value.LineageFailure
	Nodes     []LineageNode
	Edges     []LineageEdge
	Warnings  []string
}

// PullArtifact is the complete datasource persistence request.
type PullArtifact struct {
	Workspace, Filename, Name, TableauID, Environment, Site, ServerOrigin, SiteLUID, ProjectName, ProjectID string
	Content                                                                                                 []byte
	Lineage                                                                                                 Lineage
	Overwrite                                                                                               bool
}

// PullArtifactResult identifies one materialized datasource artifact.
type PullArtifactResult struct {
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

// PullOutput retains bounded pull details before projection.
type PullOutput struct {
	Source     *value.SourceContext `json:"source,omitempty"`
	Preview    *value.AcquisitionPlan
	Workspace  string `json:"workspace"`
	Status     string
	Datasource Record
	Artifact   PullArtifactResult
	Warnings   []string
	RequestID  string
	Help       []string
	// compactWarnings retains native artifact warnings separately from optional lineage.
	compactWarnings []string
}

type PullCompactArtifact struct {
	Workspace         string `json:"workspace"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	SourceLUID        string `json:"source_luid"`
	Path              string `json:"path"`
	CanonicalPath     string `json:"canonical_path,omitempty"`
	CompositionStatus string `json:"composition_status"`
}

type PullCompactResult struct {
	Source          *value.SourceContext `json:"source,omitempty"`
	Status          string               `json:"status"`
	Datasource      pullDatasource       `json:"datasource"`
	Artifact        PullCompactArtifact  `json:"artifact"`
	Warnings        []string             `json:"warnings,omitempty"`
	WarningsOmitted int                  `json:"warnings_omitted,omitempty"`
	Details         string               `json:"details"`
	Help            []string             `json:"help"`
}

type PullFullArtifact struct {
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

type PullFullResult struct {
	Source          *value.SourceContext `json:"source,omitempty"`
	Status          string               `json:"status"`
	Datasource      pullDatasource       `json:"datasource"`
	Artifact        PullFullArtifact     `json:"artifact"`
	Warnings        []string             `json:"warnings,omitempty"`
	WarningsOmitted int                  `json:"warnings_omitted,omitempty"`
	RequestID       string               `json:"tableau_request_id,omitempty"`
	Help            []string             `json:"help"`
}

func (o PullOutput) CompactOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	warningSource := o.compactWarnings
	if warningSource == nil {
		warningSource = o.Warnings
	}
	warnings, omitted := pullBoundedWarnings(warningSource)
	return PullCompactResult{Source: o.Source, Status: o.Status, Datasource: pullIdentity(o.Datasource), Artifact: PullCompactArtifact{Workspace: o.Workspace, Kind: "datasource", Name: o.Datasource.Name, SourceLUID: o.Datasource.LUID, Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath, CompositionStatus: o.Artifact.CompositionStatus}, Warnings: warnings, WarningsOmitted: omitted, Details: "--full", Help: o.Help}
}

func (o PullOutput) FullOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	warnings, omitted := pullBoundedWarnings(o.Warnings)
	artifact := PullFullArtifact{Workspace: o.Workspace, Kind: "datasource", Name: o.Datasource.Name, SourceLUID: o.Datasource.LUID, Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath, BaselineFingerprint: o.Artifact.BaselineFingerprint, LineagePath: o.Artifact.LineagePath, LineageStatus: o.Artifact.LineageStatus, CompositionStatus: o.Artifact.CompositionStatus, ParentDataSourceURLs: append([]string(nil), o.Artifact.ParentDataSourceURLs...)}
	if o.Artifact.CountsKnown {
		nodes, edges := o.Artifact.NodeCount, o.Artifact.EdgeCount
		artifact.NodeCount, artifact.EdgeCount = &nodes, &edges
	}
	return PullFullResult{Source: o.Source, Status: o.Status, Datasource: pullIdentity(o.Datasource), Artifact: artifact, Warnings: warnings, WarningsOmitted: omitted, RequestID: o.RequestID, Help: o.Help}
}

const pullWarningLimit = 20

func pullBoundedWarnings(values []string) ([]string, int) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, min(len(values), pullWarningLimit))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		if len(result) < pullWarningLimit {
			result = append(result, value)
		}
	}
	return result, max(0, len(seen)-len(result))
}

type pullDatasource struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
}
