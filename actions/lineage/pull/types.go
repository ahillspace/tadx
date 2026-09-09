package pull

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	// FullNodeLimit bounds expanded command output.
	FullNodeLimit = 100
	// FullEdgeLimit bounds expanded command output.
	FullEdgeLimit = 200
)

// Input selects one authoritative resource and bounded lineage scope.
type Input struct {
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName string
	Environment   string
	Site          string
	ServerOrigin  string
	SiteLUID      string
	Workspace     string
	Kind          string
	Selector      identity.Selector
	Direction     string
	Depth         int
	Overwrite     bool
}

// SetSelector records one exact CLI selector without exposing identity plumbing to Cobra.
func (i *Input) SetSelector(luid, name, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), Name: name, ProjectPath: projectPath}
}

// Resource is one exact REST-authoritative root.
type Resource struct {
	Kind        string `json:"kind"`
	LUID        string `json:"luid"`
	MetadataID  string `json:"metadata_id,omitempty"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
}

// CaptureRequest selects bounded Metadata lineage for an exact REST LUID.
type CaptureRequest struct {
	Kind      string
	RESTLUID  string
	Direction string
	Depth     int
}

// Node preserves distinct Metadata and REST identities.
type Node = value.LineageNode

// Edge is one factual directed relationship.
type Edge = value.LineageEdge

// Graph is one bounded lineage capture.
type Graph struct {
	RootMetadataID string
	Complete       bool
	Nodes          []Node
	Edges          []Edge
	Warnings       []string
	RequestIDs     []string
}

// Artifact is one metadata-only lineage write request.
type Artifact struct {
	Workspace       string
	Resource        Resource
	Environment     string
	Site            string
	ServerOrigin    string
	SiteLUID        string
	Direction       string
	Depth           int
	Complete        bool
	CountsKnown     bool
	Nodes           []Node
	Edges           []Edge
	Warnings        []string
	WarningsOmitted int
	RequestIDs      []string
	Overwrite       bool
}

// ArtifactResult identifies one materialized metadata-only artifact.
type ArtifactResult struct {
	Path        string `json:"path"`
	LineagePath string `json:"lineage_path"`
	Fingerprint string `json:"fingerprint"`
}

// Provenance identifies the source without conflating REST and Metadata IDs.
type Provenance struct {
	Environment  string `json:"environment,omitempty"`
	Site         string `json:"site,omitempty"`
	ServerOrigin string `json:"server_origin,omitempty"`
	SiteLUID     string `json:"site_luid,omitempty"`
}

// ArtifactSummary is the compact metadata-only artifact projection.
type ArtifactSummary struct {
	Path      string `json:"path"`
	Direction string `json:"direction"`
	Depth     int    `json:"depth"`
	Complete  bool   `json:"complete"`
	NodeCount *int   `json:"node_count,omitempty"`
	EdgeCount *int   `json:"edge_count,omitempty"`
}

// FullArtifact adds the canonical sidecar path and fingerprint.
type FullArtifact struct {
	Path        string `json:"path"`
	LineagePath string `json:"lineage_path"`
	Fingerprint string `json:"fingerprint"`
	Direction   string `json:"direction"`
	Depth       int    `json:"depth"`
	Complete    bool   `json:"complete"`
	NodeCount   *int   `json:"node_count,omitempty"`
	EdgeCount   *int   `json:"edge_count,omitempty"`
}

// Output retains complete bounded details before projection.
type Output struct {
	Status            string
	Resource          Resource
	Artifact          ArtifactResult
	Direction         string
	Depth             int
	Complete          bool
	CountsKnown       bool
	Nodes             []Node
	Edges             []Edge
	Warnings          []string
	WarningsOmitted   int
	Provenance        Provenance
	RequestIDs        []string
	RequestIDsOmitted int
	Help              []string
}

// CompactResult is the default exact identity and graph summary.
type CompactResult struct {
	Status          string          `json:"status"`
	Resource        Resource        `json:"resource"`
	Artifact        ArtifactSummary `json:"artifact"`
	Warnings        []string        `json:"warnings,omitempty"`
	WarningsOmitted int             `json:"warnings_omitted,omitempty"`
	Details         string          `json:"details"`
	Help            []string        `json:"help"`
}

// FullResult is the expanded bounded graph projection.
type FullResult struct {
	Status            string       `json:"status"`
	Resource          Resource     `json:"resource"`
	Artifact          FullArtifact `json:"artifact"`
	Nodes             []Node       `json:"nodes"`
	Edges             []Edge       `json:"edges"`
	OmittedNodeCount  int          `json:"omitted_node_count,omitempty"`
	OmittedEdgeCount  int          `json:"omitted_edge_count,omitempty"`
	Warnings          []string     `json:"warnings,omitempty"`
	WarningsOmitted   int          `json:"warnings_omitted,omitempty"`
	Provenance        Provenance   `json:"provenance"`
	RequestIDs        []string     `json:"request_ids,omitempty"`
	RequestIDsOmitted int          `json:"request_ids_omitted,omitempty"`
	Help              []string     `json:"help"`
}

// CompactOutput returns exact root identity and bounded graph counts.
func (o Output) CompactOutput() any {
	nodeCount, edgeCount := o.counts()
	return CompactResult{
		Status: o.Status, Resource: compactResource(o.Resource),
		Artifact: ArtifactSummary{Path: o.Artifact.Path, Direction: o.Direction, Depth: o.Depth, Complete: o.Complete, NodeCount: nodeCount, EdgeCount: edgeCount},
		Warnings: o.Warnings, WarningsOmitted: o.WarningsOmitted, Details: "--full", Help: o.Help,
	}
}

// FullOutput returns provenance, identity mapping, and a bounded graph page.
func (o Output) FullOutput() any {
	o.Resource.Kind = publicKind(o.Resource.Kind)
	nodes := append([]Node(nil), o.Nodes...)
	for i := range nodes {
		nodes[i].Kind = publicKind(nodes[i].Kind)
	}
	o.Nodes = nodes
	nodeLimit := len(o.Nodes)
	if nodeLimit > FullNodeLimit {
		nodeLimit = FullNodeLimit
	}
	edgeLimit := len(o.Edges)
	if edgeLimit > FullEdgeLimit {
		edgeLimit = FullEdgeLimit
	}
	nodeCount, edgeCount := o.counts()
	return FullResult{
		Status: o.Status, Resource: o.Resource,
		Artifact: FullArtifact{Path: o.Artifact.Path, LineagePath: o.Artifact.LineagePath, Fingerprint: o.Artifact.Fingerprint, Direction: o.Direction, Depth: o.Depth, Complete: o.Complete, NodeCount: nodeCount, EdgeCount: edgeCount},
		Nodes:    append([]Node(nil), o.Nodes[:nodeLimit]...), Edges: append([]Edge(nil), o.Edges[:edgeLimit]...),
		OmittedNodeCount: len(o.Nodes) - nodeLimit, OmittedEdgeCount: len(o.Edges) - edgeLimit,
		Warnings: o.Warnings, WarningsOmitted: o.WarningsOmitted, Provenance: o.Provenance,
		RequestIDs: o.RequestIDs, RequestIDsOmitted: o.RequestIDsOmitted, Help: o.Help,
	}
}

func (o Output) counts() (*int, *int) {
	if !o.CountsKnown {
		return nil, nil
	}
	nodes, edges := len(o.Nodes), len(o.Edges)
	return &nodes, &edges
}

func compactResource(resource Resource) Resource {
	resource.Kind = publicKind(resource.Kind)
	resource.MetadataID = ""
	return resource
}

func publicKind(kind string) string {
	if kind == "published_datasource" {
		return "datasource"
	}
	return kind
}
