// Package lineage normalizes bounded Tableau Metadata lineage results.
package lineage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	tableaumetadata "github.com/ahillspace/tadx/internal/tableau/metadata"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	// MaxNodes bounds one persisted graph.
	MaxNodes = 500
	// MaxEdges bounds one persisted directed graph.
	MaxEdges = 1000
	// MaxWarnings bounds safe diagnostics.
	MaxWarnings = 20
)

// Client is the narrow docs-only Metadata lineage seam.
type Client interface {
	CaptureLineage(context.Context, tableaumetadata.CaptureRequest) (tableaumetadata.Capture, error)
}

// Request selects one exact authoritative REST root.
type Request struct {
	Kind      string
	RESTLUID  string
	Direction string
	Depth     int
}

// Node preserves distinct Metadata and REST identities.
type Node = value.LineageNode

// Edge is one unique factual directed relationship.
type Edge = value.LineageEdge

// Graph is one deterministic bounded lineage result.
type Graph struct {
	RootRESTLUID   string
	RootMetadataID string
	Direction      string
	Depth          int
	Complete       bool
	Nodes          []Node
	Edges          []Edge
	Warnings       []string
	RequestIDs     []string
}

// Adapter isolates Metadata API normalization from actions.
type Adapter struct{ client Client }

// NewAdapter creates a lineage resource adapter.
func NewAdapter(client Client) *Adapter { return &Adapter{client: client} }

// Capture returns a deterministic graph within the frozen artifact bounds.
func (a *Adapter) Capture(ctx context.Context, input Request) (Graph, error) {
	if a == nil || a.client == nil {
		return Graph{}, errors.New("lineage resource adapter is not configured")
	}
	request, err := tableaumetadata.ValidateLineageRequest(tableaumetadata.CaptureRequest{
		Kind: tableaumetadata.ResourceKind(strings.TrimSpace(input.Kind)), RESTLUID: input.RESTLUID,
		Direction: tableaumetadata.Direction(strings.TrimSpace(input.Direction)), Depth: input.Depth,
	})
	if err != nil {
		return Graph{}, err
	}
	capture, err := a.client.CaptureLineage(ctx, request)
	if err != nil {
		return Graph{}, err
	}
	return normalize(request, capture)
}

func normalize(request tableaumetadata.CaptureRequest, capture tableaumetadata.Capture) (Graph, error) {
	rootRESTLUID := strings.TrimSpace(capture.RootRESTLUID)
	rootMetadataID := strings.TrimSpace(capture.RootMetadataID)
	if rootRESTLUID != "" && rootRESTLUID != request.RESTLUID {
		return Graph{}, fmt.Errorf("lineage root returned REST LUID %q, expected %q", rootRESTLUID, request.RESTLUID)
	}
	if capture.Complete && (rootRESTLUID == "" || rootMetadataID == "") {
		return Graph{}, errors.New("complete lineage omitted the root REST LUID or Metadata ID")
	}

	byMetadataID := make(map[string]Node, len(capture.Nodes))
	for _, raw := range capture.Nodes {
		node := Node{MetadataID: strings.TrimSpace(raw.MetadataID), Kind: strings.TrimSpace(raw.Kind), RESTLUID: strings.TrimSpace(raw.RESTLUID), Name: strings.TrimSpace(raw.Name)}
		if node.MetadataID == "" || node.Kind == "" {
			return Graph{}, errors.New("lineage node omitted its Metadata ID or kind")
		}
		if current, exists := byMetadataID[node.MetadataID]; exists {
			if current != node {
				return Graph{}, fmt.Errorf("lineage Metadata ID %q has conflicting records", node.MetadataID)
			}
			continue
		}
		byMetadataID[node.MetadataID] = node
	}
	if rootMetadataID != "" {
		root, exists := byMetadataID[rootMetadataID]
		if !exists {
			return Graph{}, fmt.Errorf("lineage root Metadata ID %q is absent from nodes", rootMetadataID)
		}
		if root.RESTLUID != request.RESTLUID {
			return Graph{}, errors.New("lineage root Metadata ID does not map to the authoritative REST LUID")
		}
		if root.Kind != string(request.Kind) {
			return Graph{}, errors.New("lineage root Metadata ID does not map to the requested resource kind")
		}
	}

	nodes := make([]Node, 0, len(byMetadataID))
	for _, node := range byMetadataID {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].MetadataID == rootMetadataID {
			return nodes[j].MetadataID != rootMetadataID
		}
		if nodes[j].MetadataID == rootMetadataID {
			return false
		}
		return nodeKey(nodes[i]) < nodeKey(nodes[j])
	})

	allNodeIDs := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		allNodeIDs[node.MetadataID] = struct{}{}
	}
	byEdge := make(map[string]Edge, len(capture.Edges))
	for _, raw := range capture.Edges {
		edge := Edge{FromMetadataID: strings.TrimSpace(raw.FromMetadataID), ToMetadataID: strings.TrimSpace(raw.ToMetadataID), Relationship: strings.TrimSpace(raw.Relationship)}
		if edge.FromMetadataID == "" || edge.ToMetadataID == "" || edge.Relationship == "" {
			return Graph{}, errors.New("lineage edge omitted an endpoint or relationship")
		}
		if _, exists := allNodeIDs[edge.FromMetadataID]; !exists {
			return Graph{}, fmt.Errorf("lineage edge references unknown source Metadata ID %q", edge.FromMetadataID)
		}
		if _, exists := allNodeIDs[edge.ToMetadataID]; !exists {
			return Graph{}, fmt.Errorf("lineage edge references unknown destination Metadata ID %q", edge.ToMetadataID)
		}
		byEdge[edgeKey(edge)] = edge
	}
	edges := make([]Edge, 0, len(byEdge))
	for _, edge := range byEdge {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool { return edgeKey(edges[i]) < edgeKey(edges[j]) })

	complete := capture.Complete
	generatedWarnings := make([]string, 0, 2)
	if len(nodes) > MaxNodes {
		nodes = nodes[:MaxNodes]
		complete = false
		generatedWarnings = append(generatedWarnings, "Lineage nodes exceeded the 500-node artifact limit; the stored graph is incomplete.")
	}
	retained := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		retained[node.MetadataID] = struct{}{}
	}
	filteredEdges := edges[:0]
	for _, edge := range edges {
		_, fromExists := retained[edge.FromMetadataID]
		_, toExists := retained[edge.ToMetadataID]
		if fromExists && toExists {
			filteredEdges = append(filteredEdges, edge)
		} else {
			complete = false
		}
	}
	edges = filteredEdges
	if len(edges) > MaxEdges {
		edges = edges[:MaxEdges]
		complete = false
		generatedWarnings = append(generatedWarnings, "Lineage edges exceeded the 1000-edge artifact limit; the stored graph is incomplete.")
	}

	warnings := normalizeStrings(append(generatedWarnings, capture.Warnings...), MaxWarnings)
	requestIDs := normalizeStrings(capture.RequestIDs, MaxWarnings)
	return Graph{RootRESTLUID: request.RESTLUID, RootMetadataID: rootMetadataID, Direction: string(request.Direction), Depth: request.Depth, Complete: complete, Nodes: nodes, Edges: edges, Warnings: warnings, RequestIDs: requestIDs}, nil
}

func nodeKey(node Node) string {
	return node.MetadataID + "\x00" + node.Kind + "\x00" + node.RESTLUID + "\x00" + node.Name
}

func edgeKey(edge Edge) string {
	return edge.FromMetadataID + "\x00" + edge.ToMetadataID + "\x00" + edge.Relationship
}

func normalizeStrings(values []string, limit int) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}
