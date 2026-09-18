package pull

import "github.com/ahillspace/tadx/internal/value"

import (
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

type Input struct {
	Preview bool
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName                                        string
	Environment, Site, ServerOrigin, SiteLUID, Workspace string
	Selector                                             identity.Selector
	Overwrite                                            bool
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

type Flow struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	FileType    string `json:"file_type,omitempty"`
}
type Download struct {
	Filename         string
	Content          []byte
	TableauRequestID string
}
type LineageRequest struct {
	Kind, RESTLUID, Direction string
	Depth                     int
}
type LineageNode struct{ MetadataID, Kind, RESTLUID, Name string }
type LineageEdge struct{ FromMetadataID, ToMetadataID, Relationship string }
type Lineage struct {
	Complete  bool
	Direction string
	Depth     int
	Failure   *value.LineageFailure
	Nodes     []LineageNode
	Edges     []LineageEdge
	Warnings  []string
}
type Artifact struct {
	Workspace, Filename, Name, TableauID, Environment, Site, ServerOrigin, SiteLUID, ProjectName, ProjectID, FileType string
	Content                                                                                                           []byte
	Lineage                                                                                                           Lineage
	Overwrite                                                                                                         bool
}
type ArtifactResult struct {
	Path                string   `json:"path"`
	CanonicalPath       string   `json:"canonical_path,omitempty"`
	BaselineFingerprint string   `json:"baseline_fingerprint,omitempty"`
	LineagePath         string   `json:"lineage_path,omitempty"`
	LineageStatus       string   `json:"lineage_status"`
	NodeCount           int      `json:"node_count"`
	EdgeCount           int      `json:"edge_count"`
	CountsKnown         bool     `json:"-"`
	Warnings            []string `json:"-"`
}
type Output struct {
	Source    *value.SourceContext `json:"source,omitempty"`
	Preview   *value.AcquisitionPlan
	Workspace string `json:"workspace"`
	Status    string
	Flow      Flow
	Artifact  ArtifactResult
	Warnings  []string
	RequestID string
	Help      []string
	// compactWarnings retains native artifact warnings separately from optional lineage.
	compactWarnings []string
}
type CompactArtifact struct {
	Workspace     string `json:"workspace"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	SourceLUID    string `json:"source_luid"`
	Path          string `json:"path"`
	CanonicalPath string `json:"canonical_path,omitempty"`
}
type CompactResult struct {
	Source          *value.SourceContext `json:"source,omitempty"`
	Status          string               `json:"status"`
	Flow            Flow                 `json:"flow"`
	Artifact        CompactArtifact      `json:"artifact"`
	Warnings        []string             `json:"warnings,omitempty"`
	WarningsOmitted int                  `json:"warnings_omitted,omitempty"`
	Details         string               `json:"details"`
	Help            []string             `json:"help"`
}
type FullArtifact struct {
	Workspace           string `json:"workspace"`
	Kind                string `json:"kind"`
	Name                string `json:"name"`
	SourceLUID          string `json:"source_luid"`
	Path                string `json:"path"`
	CanonicalPath       string `json:"canonical_path,omitempty"`
	BaselineFingerprint string `json:"baseline_fingerprint,omitempty"`
	LineagePath         string `json:"lineage_path,omitempty"`
	LineageStatus       string `json:"lineage_status"`
	NodeCount           *int   `json:"node_count,omitempty"`
	EdgeCount           *int   `json:"edge_count,omitempty"`
}
type FullResult struct {
	Source          *value.SourceContext `json:"source,omitempty"`
	Status          string               `json:"status"`
	Flow            Flow                 `json:"flow"`
	Artifact        FullArtifact         `json:"artifact"`
	Warnings        []string             `json:"warnings,omitempty"`
	WarningsOmitted int                  `json:"warnings_omitted,omitempty"`
	RequestID       string               `json:"tableau_request_id,omitempty"`
	Help            []string             `json:"help"`
}

func (o Output) CompactOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	warningSource := o.compactWarnings
	if warningSource == nil {
		warningSource = o.Warnings
	}
	warnings, omitted := boundedWarnings(warningSource)
	return CompactResult{Source: o.Source, Status: o.Status, Flow: o.Flow, Artifact: CompactArtifact{Workspace: o.Workspace, Kind: "flow", Name: o.Flow.Name, SourceLUID: o.Flow.LUID, Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath}, Warnings: warnings, WarningsOmitted: omitted, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	warnings, omitted := boundedWarnings(o.Warnings)
	artifact := FullArtifact{Workspace: o.Workspace, Kind: "flow", Name: o.Flow.Name, SourceLUID: o.Flow.LUID,
		Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath,
		BaselineFingerprint: o.Artifact.BaselineFingerprint, LineagePath: o.Artifact.LineagePath,
		LineageStatus: o.Artifact.LineageStatus,
	}
	if o.Artifact.CountsKnown {
		nodes, edges := o.Artifact.NodeCount, o.Artifact.EdgeCount
		artifact.NodeCount, artifact.EdgeCount = &nodes, &edges
	}
	return FullResult{Source: o.Source, Status: o.Status, Flow: o.Flow, Artifact: artifact, Warnings: warnings, WarningsOmitted: omitted, RequestID: o.RequestID, Help: o.Help}
}

const warningLimit = 20

func boundedWarnings(values []string) ([]string, int) {
	seen := make(map[string]struct{}, len(values))
	bounded := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		bounded = append(bounded, value)
	}
	if len(bounded) <= warningLimit {
		return bounded, 0
	}
	return bounded[:warningLimit], len(bounded) - warningLimit
}
