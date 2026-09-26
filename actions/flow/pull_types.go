package flow

import (
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type PullInput struct {
	Preview bool
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName                                        string
	Environment, Site, ServerOrigin, SiteLUID, Workspace string
	Selector                                             identity.Selector
	Overwrite                                            bool
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *PullInput) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// pullFlow preserves the original artifact acquisition projection.
type pullFlow struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	ProjectLUID string `json:"project_luid,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	FileType    string `json:"file_type,omitempty"`
}

type PullDownload struct {
	Filename         string
	Content          []byte
	TableauRequestID string
}
type PullLineageRequest struct {
	Kind, RESTLUID, Direction string
	Depth                     int
}
type PullLineageNode = value.LineageNode
type PullLineageEdge = value.LineageEdge
type PullLineage struct {
	Complete  bool
	Direction string
	Depth     int
	Failure   *value.LineageFailure
	Nodes     []PullLineageNode
	Edges     []PullLineageEdge
	Warnings  []string
}
type PullArtifact struct {
	Workspace, Filename, Name, TableauID, Environment, Site, ServerOrigin, SiteLUID, ProjectName, ProjectID, FileType string
	Content                                                                                                           []byte
	Lineage                                                                                                           PullLineage
	Overwrite                                                                                                         bool
}
type PullArtifactResult struct {
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
type PullOutput struct {
	Source    *value.SourceContext `json:"source,omitempty"`
	Preview   *value.AcquisitionPlan
	Workspace string `json:"workspace"`
	Status    string
	Flow      Record
	Artifact  PullArtifactResult
	Warnings  []string
	RequestID string
	Help      []string
	// compactWarnings retains native artifact warnings separately from optional lineage.
	compactWarnings []string
}
type PullCompactArtifact struct {
	Workspace     string `json:"workspace"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	SourceLUID    string `json:"source_luid"`
	Path          string `json:"path"`
	CanonicalPath string `json:"canonical_path,omitempty"`
}
type PullCompactResult struct {
	Source          *value.SourceContext `json:"source,omitempty"`
	Status          string               `json:"status"`
	Flow            pullFlow             `json:"flow"`
	Artifact        PullCompactArtifact  `json:"artifact"`
	Warnings        []string             `json:"warnings,omitempty"`
	WarningsOmitted int                  `json:"warnings_omitted,omitempty"`
	Details         string               `json:"details"`
	Help            []string             `json:"help"`
}
type PullFullArtifact struct {
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
type PullFullResult struct {
	Source          *value.SourceContext `json:"source,omitempty"`
	Status          string               `json:"status"`
	Flow            pullFlow             `json:"flow"`
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
	return PullCompactResult{Source: o.Source, Status: o.Status, Flow: pullIdentity(o.Flow), Artifact: PullCompactArtifact{Workspace: o.Workspace, Kind: "flow", Name: o.Flow.Name, SourceLUID: o.Flow.LUID, Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath}, Warnings: warnings, WarningsOmitted: omitted, Details: "--full", Help: o.Help}
}
func (o PullOutput) FullOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	warnings, omitted := pullBoundedWarnings(o.Warnings)
	artifact := PullFullArtifact{Workspace: o.Workspace, Kind: "flow", Name: o.Flow.Name, SourceLUID: o.Flow.LUID,
		Path: o.Artifact.Path, CanonicalPath: o.Artifact.CanonicalPath,
		BaselineFingerprint: o.Artifact.BaselineFingerprint, LineagePath: o.Artifact.LineagePath,
		LineageStatus: o.Artifact.LineageStatus,
	}
	if o.Artifact.CountsKnown {
		nodes, edges := o.Artifact.NodeCount, o.Artifact.EdgeCount
		artifact.NodeCount, artifact.EdgeCount = &nodes, &edges
	}
	return PullFullResult{Source: o.Source, Status: o.Status, Flow: pullIdentity(o.Flow), Artifact: artifact, Warnings: warnings, WarningsOmitted: omitted, RequestID: o.RequestID, Help: o.Help}
}

func pullIdentity(item Record) pullFlow {
	return pullFlow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, FileType: item.FileType}
}

const pullWarningLimit = 20

func pullBoundedWarnings(values []string) ([]string, int) {
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
	if len(bounded) <= pullWarningLimit {
		return bounded, 0
	}
	return bounded[:pullWarningLimit], len(bounded) - pullWarningLimit
}
