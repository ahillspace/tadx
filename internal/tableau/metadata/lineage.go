package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau"
)

const (
	// DefaultLineageDepth is the direct-neighbor capture depth.
	DefaultLineageDepth = 1
	// MaxLineageDepth bounds transitive capture.
	MaxLineageDepth = 3
	// LineagePageSize is the frozen maximum Metadata connection page size.
	LineagePageSize = 100
	// MaxLineageNodes bounds transport work before artifact normalization.
	MaxLineageNodes = 500
	// MaxLineageEdges bounds transport work before artifact normalization.
	MaxLineageEdges    = 1000
	maxLineageWarnings = 20
)

// ResourceKind is one accepted authoritative REST root kind.
type ResourceKind string

const (
	KindWorkbook            ResourceKind = "workbook"
	KindPublishedDatasource ResourceKind = "published_datasource"
	KindFlow                ResourceKind = "flow"
)

// Direction selects factual directed relationships around the root.
type Direction string

const (
	DirectionUpstream   Direction = "upstream"
	DirectionDownstream Direction = "downstream"
	DirectionBoth       Direction = "both"
)

// CaptureRequest selects one bounded lineage capture.
type CaptureRequest struct {
	Kind      ResourceKind
	RESTLUID  string
	Direction Direction
	Depth     int
	PageSize  int
}

// Node keeps the Metadata ID separate from the optional REST LUID.
type Node struct {
	MetadataID string
	Kind       string
	RESTLUID   string
	Name       string
}

// Edge is one factual directed relationship.
type Edge struct {
	FromMetadataID string
	ToMetadataID   string
	Relationship   string
}

// Capture is one transport-neutral Metadata API result.
type Capture struct {
	RootRESTLUID   string
	RootMetadataID string
	Complete       bool
	Nodes          []Node
	Edges          []Edge
	Warnings       []string
	RequestIDs     []string
}

// LineageReader captures bounded factual Metadata API relationships.
type LineageReader interface {
	CaptureLineage(context.Context, CaptureRequest) (Capture, error)
}

type lineageRelation struct {
	field      string
	targetKind ResourceKind
	direction  Direction
	linked     bool
}

var lineageRelations = map[ResourceKind]map[Direction][]lineageRelation{
	KindWorkbook: {
		DirectionUpstream: {
			{field: "upstreamDatasourcesConnection", targetKind: KindPublishedDatasource, direction: DirectionUpstream},
		},
	},
	KindPublishedDatasource: {
		DirectionUpstream: {
			{field: "upstreamDatasourcesConnection", targetKind: KindPublishedDatasource, direction: DirectionUpstream},
			{field: "upstreamFlowsConnection", targetKind: KindFlow, direction: DirectionUpstream},
		},
		DirectionDownstream: {
			{field: "downstreamDatasourcesConnection", targetKind: KindPublishedDatasource, direction: DirectionDownstream},
			{field: "downstreamFlowsConnection", targetKind: KindFlow, direction: DirectionDownstream},
			{field: "downstreamWorkbooksConnection", targetKind: KindWorkbook, direction: DirectionDownstream},
		},
	},
	KindFlow: {
		DirectionUpstream: {
			{field: "upstreamDatasourcesConnection", targetKind: KindPublishedDatasource, direction: DirectionUpstream},
			{field: "upstreamLinkedFlowsConnection", targetKind: KindFlow, direction: DirectionUpstream, linked: true},
		},
		DirectionDownstream: {
			{field: "downstreamDatasourcesConnection", targetKind: KindPublishedDatasource, direction: DirectionDownstream},
			{field: "downstreamLinkedFlowsConnection", targetKind: KindFlow, direction: DirectionDownstream, linked: true},
			{field: "downstreamWorkbooksConnection", targetKind: KindWorkbook, direction: DirectionDownstream},
		},
	},
}

type lineageFrontier struct {
	node  Node
	depth int
}

// CaptureLineage returns one deterministic graph of factual, bounded relationships.
func (c *Client) CaptureLineage(ctx context.Context, input CaptureRequest) (Capture, error) {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return Capture{}, errors.New("Tableau metadata client is not configured")
	}
	request, err := ValidateLineageRequest(input)
	if err != nil {
		return Capture{}, err
	}

	root, response, warnings, err := c.resolveLineageRoot(ctx, request)
	if err != nil {
		return Capture{}, err
	}
	capture := Capture{
		RootRESTLUID: request.RESTLUID, RootMetadataID: root.MetadataID, Complete: len(warnings) == 0,
		Nodes: []Node{root}, Warnings: append([]string(nil), warnings...), RequestIDs: requestIDs(response),
	}
	if len(warnings) > 0 {
		return normalizeCapture(capture), nil
	}

	nodes := map[string]Node{root.MetadataID: root}
	restIdentities := map[string]string{lineageRESTKey(root): root.MetadataID}
	edges := make(map[string]Edge)
	frontier := []lineageFrontier{{node: root, depth: 0}}
	expanded := make(map[string]struct{})
	bounded := false
	for len(frontier) > 0 && !bounded {
		current := frontier[0]
		frontier = frontier[1:]
		if current.depth >= request.Depth {
			continue
		}
		if _, exists := expanded[current.node.MetadataID]; exists {
			continue
		}
		expanded[current.node.MetadataID] = struct{}{}
		for _, relation := range relationsFor(current.node.Kind, request.Direction) {
			neighbors, responses, relationWarnings, err := c.readLineageRelation(ctx, request, current.node, relation)
			for _, queryResponse := range responses {
				capture.RequestIDs = append(capture.RequestIDs, queryResponse.TableauRequestID)
			}
			capture.Warnings = append(capture.Warnings, relationWarnings...)
			if len(relationWarnings) > 0 {
				capture.Complete = false
			}
			if err != nil {
				return Capture{}, err
			}
			for _, neighbor := range neighbors {
				if err := addLineageNode(nodes, restIdentities, neighbor); err != nil {
					return Capture{}, protocolError(lastResponse(responses, response), "%v", err)
				}
				if len(nodes) > MaxLineageNodes {
					delete(nodes, neighbor.MetadataID)
					delete(restIdentities, lineageRESTKey(neighbor))
					capture.Complete = false
					capture.Warnings = append(capture.Warnings, "Lineage exceeded the 500-node transport limit; the capture is incomplete.")
					bounded = true
					break
				}
				edge := lineageEdge(current.node, neighbor, relation.direction)
				edges[lineageEdgeKey(edge)] = edge
				if len(edges) > MaxLineageEdges {
					delete(edges, lineageEdgeKey(edge))
					capture.Complete = false
					capture.Warnings = append(capture.Warnings, "Lineage exceeded the 1000-edge transport limit; the capture is incomplete.")
					bounded = true
					break
				}
				if current.depth+1 < request.Depth {
					frontier = append(frontier, lineageFrontier{node: neighbor, depth: current.depth + 1})
				}
			}
			if bounded {
				break
			}
		}
	}
	capture.Nodes = capture.Nodes[:0]
	for _, node := range nodes {
		capture.Nodes = append(capture.Nodes, node)
	}
	for _, edge := range edges {
		capture.Edges = append(capture.Edges, edge)
	}
	return normalizeCapture(capture), nil
}

func (c *Client) resolveLineageRoot(ctx context.Context, request CaptureRequest) (Node, tableau.Response, []string, error) {
	query, err := StaticLineageRootQuery(request.Kind)
	if err != nil {
		return Node{}, tableau.Response{}, nil, err
	}
	response, envelope, warnings, err := c.queryLineage(ctx, query, map[string]any{"rootLuid": request.RESTLUID, "after": nil, "pageSize": 2})
	if err != nil {
		return Node{}, response, nil, err
	}
	connection := envelope.Data.connection(request.Kind)
	root, err := exactLineageRoot(response, connection, request.Kind, request.RESTLUID)
	if err != nil {
		return Node{}, response, nil, err
	}
	return root, response, warnings, nil
}

func (c *Client) readLineageRelation(ctx context.Context, request CaptureRequest, root Node, relation lineageRelation) ([]Node, []tableau.Response, []string, error) {
	query, err := lineageRelationQuery(ResourceKind(root.Kind), relation)
	if err != nil {
		return nil, nil, nil, err
	}
	var after any
	total := -1
	seenCursors := make(map[string]struct{})
	seenNodes := make(map[string]struct{})
	result := make([]Node, 0)
	responses := make([]tableau.Response, 0, 1)
	warnings := make([]string, 0)
	for {
		response, envelope, pageWarnings, queryErr := c.queryLineage(ctx, query, map[string]any{"rootLuid": root.RESTLUID, "after": after, "pageSize": request.PageSize})
		responses = append(responses, response)
		warnings = append(warnings, pageWarnings...)
		if queryErr != nil {
			return nil, responses, warnings, queryErr
		}
		connection := envelope.Data.connection(ResourceKind(root.Kind))
		returnedRoot, rootErr := exactLineageRoot(response, connection, ResourceKind(root.Kind), root.RESTLUID)
		if rootErr != nil {
			return nil, responses, warnings, rootErr
		}
		if returnedRoot != root {
			return nil, responses, warnings, protocolError(response, "lineage root REST LUID %q changed identity while paging %s", root.RESTLUID, relation.field)
		}
		relationship := (*connection.Nodes)[0].relation(relation)
		if relationship == nil || relationship.TotalCount == nil || relationship.PageInfo == nil || relationship.Nodes == nil {
			return nil, responses, warnings, protocolError(response, "%s omitted totalCount, pageInfo, or nodes", relation.field)
		}
		if *relationship.TotalCount < 0 {
			return nil, responses, warnings, protocolError(response, "%s returned a negative totalCount", relation.field)
		}
		if total < 0 {
			total = *relationship.TotalCount
		} else if total != *relationship.TotalCount {
			return nil, responses, warnings, protocolError(response, "%s changed totalCount from %d to %d while paging", relation.field, total, *relationship.TotalCount)
		}
		for _, raw := range *relationship.Nodes {
			neighborRaw := raw
			if relation.linked {
				if raw.Asset == nil {
					return nil, responses, warnings, protocolError(response, "%s returned a linked flow without an asset", relation.field)
				}
				neighborRaw = *raw.Asset
			}
			neighbor := Node{MetadataID: strings.TrimSpace(neighborRaw.ID), Kind: string(relation.targetKind), RESTLUID: strings.TrimSpace(neighborRaw.LUID), Name: strings.TrimSpace(neighborRaw.Name)}
			if neighbor.MetadataID == "" || neighbor.RESTLUID == "" {
				return nil, responses, warnings, protocolError(response, "%s returned a %s without distinct Metadata and REST identities", relation.field, relation.targetKind)
			}
			if _, exists := seenNodes[neighbor.MetadataID]; exists {
				return nil, responses, warnings, protocolError(response, "%s repeated Metadata ID %q within one connection", relation.field, neighbor.MetadataID)
			}
			seenNodes[neighbor.MetadataID] = struct{}{}
			result = append(result, neighbor)
		}
		if len(result) > total {
			return nil, responses, warnings, protocolError(response, "%s returned more nodes than totalCount", relation.field)
		}
		if relationship.PageInfo.HasNextPage == nil {
			return nil, responses, warnings, protocolError(response, "%s omitted pageInfo.hasNextPage", relation.field)
		}
		if !*relationship.PageInfo.HasNextPage {
			if len(result) != total && len(warnings) == 0 {
				return nil, responses, warnings, protocolError(response, "%s ended at %d of %d nodes", relation.field, len(result), total)
			}
			return result, responses, warnings, nil
		}
		cursor := strings.TrimSpace(relationship.PageInfo.EndCursor)
		if cursor == "" {
			return nil, responses, warnings, protocolError(response, "%s has another page without an end cursor", relation.field)
		}
		if _, exists := seenCursors[cursor]; exists {
			return nil, responses, warnings, protocolError(response, "%s repeated cursor %q", relation.field, cursor)
		}
		seenCursors[cursor] = struct{}{}
		after = cursor
	}
}

func (c *Client) queryLineage(ctx context.Context, query string, variables map[string]any) (tableau.Response, lineageEnvelope, []string, error) {
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return tableau.Response{}, lineageEnvelope{}, nil, fmt.Errorf("encode Tableau lineage query: %w", err)
	}
	response, err := c.transport.Do(ctx, c.session, tableau.Request{
		Method: http.MethodPost, ServerURL: c.serverURL, Path: "/api/metadata/graphql",
		Operation: queryOperation, Body: body, ContentType: "application/json", Accept: "application/json",
		MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		return tableau.Response{}, lineageEnvelope{}, nil, err
	}
	var envelope lineageEnvelope
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return response, lineageEnvelope{}, nil, tableau.NewProtocolError(queryOperation, response, fmt.Errorf("decode Metadata API lineage response: %w", err), false)
	}
	topWarnings, err := decodeWarnings(envelope.Warnings)
	if err != nil {
		return response, lineageEnvelope{}, nil, tableau.NewProtocolError(queryOperation, response, fmt.Errorf("decode Metadata API lineage warnings: %w", err), false)
	}
	issues := append(append([]graphqlIssue(nil), envelope.Errors...), topWarnings...)
	if len(issues) == 0 {
		return response, envelope, nil, nil
	}
	incomplete := make([]string, 0, len(issues))
	fatal := make([]graphqlIssue, 0, len(issues))
	for _, issue := range issues {
		if isIncompleteLineageIssue(issue) {
			if len(incomplete) < maxLineageWarnings {
				incomplete = append(incomplete, fmt.Sprintf("Metadata API reported %s; lineage is incomplete.", safeLineageIssueCode(issue)))
			}
			continue
		}
		fatal = append(fatal, issue)
	}
	if len(fatal) > 0 {
		return response, lineageEnvelope{}, nil, newGraphQLIssuesError(response, issues)
	}
	return response, envelope, incomplete, nil
}

func isIncompleteLineageIssue(issue graphqlIssue) bool {
	switch issue.code() {
	case "BACKFILL_RUNNING", "INHERITANCE_INCOMPLETE", "LINKED_RESULTS_INCOMPLETE", "MAX_PAGE_SIZE_EXCEEDED", "NODE_LIMIT_EXCEEDED", "TIME_LIMIT_EXCEEDED", "USER_VISIBILITY_IS_LIMITED":
		return true
	default:
		return strings.EqualFold(strings.TrimSpace(issue.Extensions.Severity), "WARNING")
	}
}

func safeLineageIssueCode(issue graphqlIssue) string {
	switch issue.code() {
	case "BACKFILL_RUNNING", "INHERITANCE_INCOMPLETE", "LINKED_RESULTS_INCOMPLETE", "MAX_PAGE_SIZE_EXCEEDED", "NODE_LIMIT_EXCEEDED", "TIME_LIMIT_EXCEEDED", "USER_VISIBILITY_IS_LIMITED":
		return issue.code()
	default:
		return "UNKNOWN_WARNING"
	}
}

func exactLineageRoot(response tableau.Response, connection *lineageConnection, kind ResourceKind, expectedLUID string) (Node, error) {
	if connection == nil || connection.TotalCount == nil || connection.PageInfo == nil || connection.Nodes == nil {
		return Node{}, protocolError(response, "%s lineage root connection omitted totalCount, pageInfo, or nodes", kind)
	}
	if connection.PageInfo.HasNextPage == nil {
		return Node{}, protocolError(response, "%s lineage root connection omitted pageInfo.hasNextPage", kind)
	}
	if *connection.TotalCount != 1 || len(*connection.Nodes) != 1 || *connection.PageInfo.HasNextPage {
		return Node{}, protocolError(response, "expected exactly one %s lineage root for REST LUID %q", kind, expectedLUID)
	}
	raw := (*connection.Nodes)[0]
	root := Node{MetadataID: strings.TrimSpace(raw.ID), Kind: string(kind), RESTLUID: strings.TrimSpace(raw.LUID), Name: strings.TrimSpace(raw.Name)}
	if root.MetadataID == "" || root.RESTLUID == "" {
		return Node{}, protocolError(response, "%s lineage root omitted its Metadata ID or REST LUID", kind)
	}
	if root.RESTLUID != expectedLUID {
		return Node{}, protocolError(response, "%s lineage root returned REST LUID %q, expected REST LUID %q", kind, root.RESTLUID, expectedLUID)
	}
	return root, nil
}

func relationsFor(kind string, direction Direction) []lineageRelation {
	byDirection := lineageRelations[ResourceKind(kind)]
	switch direction {
	case DirectionUpstream:
		return append([]lineageRelation(nil), byDirection[DirectionUpstream]...)
	case DirectionDownstream:
		return append([]lineageRelation(nil), byDirection[DirectionDownstream]...)
	default:
		result := append([]lineageRelation(nil), byDirection[DirectionUpstream]...)
		return append(result, byDirection[DirectionDownstream]...)
	}
}

func addLineageNode(nodes map[string]Node, restIdentities map[string]string, node Node) error {
	if current, exists := nodes[node.MetadataID]; exists {
		if current != node {
			return fmt.Errorf("Metadata ID %q has conflicting lineage identities", node.MetadataID)
		}
		return nil
	}
	key := lineageRESTKey(node)
	if currentID, exists := restIdentities[key]; exists && currentID != node.MetadataID {
		return fmt.Errorf("REST identity %q maps to Metadata IDs %q and %q", key, currentID, node.MetadataID)
	}
	nodes[node.MetadataID] = node
	restIdentities[key] = node.MetadataID
	return nil
}

func lineageRESTKey(node Node) string { return node.Kind + "\x00" + node.RESTLUID }

func lineageEdge(root, neighbor Node, direction Direction) Edge {
	if direction == DirectionUpstream {
		return Edge{FromMetadataID: neighbor.MetadataID, ToMetadataID: root.MetadataID, Relationship: string(direction)}
	}
	return Edge{FromMetadataID: root.MetadataID, ToMetadataID: neighbor.MetadataID, Relationship: string(direction)}
}

func lineageEdgeKey(edge Edge) string {
	return edge.FromMetadataID + "\x00" + edge.ToMetadataID + "\x00" + edge.Relationship
}

func normalizeCapture(capture Capture) Capture {
	sort.Slice(capture.Nodes, func(i, j int) bool {
		return lineageNodeKey(capture.Nodes[i]) < lineageNodeKey(capture.Nodes[j])
	})
	sort.Slice(capture.Edges, func(i, j int) bool {
		return lineageEdgeKey(capture.Edges[i]) < lineageEdgeKey(capture.Edges[j])
	})
	capture.Warnings = uniqueSortedLineageStrings(capture.Warnings, maxLineageWarnings)
	capture.RequestIDs = uniqueSortedLineageStrings(capture.RequestIDs, maxLineageWarnings)
	return capture
}

func lineageNodeKey(node Node) string {
	return node.MetadataID + "\x00" + node.Kind + "\x00" + node.RESTLUID + "\x00" + node.Name
}

func uniqueSortedLineageStrings(values []string, limit int) []string {
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

func requestIDs(response tableau.Response) []string {
	if strings.TrimSpace(response.TableauRequestID) == "" {
		return nil
	}
	return []string{response.TableauRequestID}
}

func lastResponse(responses []tableau.Response, fallback tableau.Response) tableau.Response {
	if len(responses) == 0 {
		return fallback
	}
	return responses[len(responses)-1]
}

// ValidateLineageRequest applies frozen defaults and rejects unsupported scope.
func ValidateLineageRequest(request CaptureRequest) (CaptureRequest, error) {
	request.RESTLUID = strings.TrimSpace(request.RESTLUID)
	if request.Kind != KindWorkbook && request.Kind != KindPublishedDatasource && request.Kind != KindFlow {
		return CaptureRequest{}, fmt.Errorf("unsupported lineage root kind %q", request.Kind)
	}
	if request.RESTLUID == "" {
		return CaptureRequest{}, errors.New("lineage root REST LUID is required")
	}
	if request.Direction == "" {
		request.Direction = DirectionBoth
	}
	if request.Direction != DirectionUpstream && request.Direction != DirectionDownstream && request.Direction != DirectionBoth {
		return CaptureRequest{}, fmt.Errorf("unsupported lineage direction %q", request.Direction)
	}
	if request.Depth == 0 {
		request.Depth = DefaultLineageDepth
	}
	if request.Depth < 1 || request.Depth > MaxLineageDepth {
		return CaptureRequest{}, fmt.Errorf("lineage depth must be between 1 and %d", MaxLineageDepth)
	}
	if request.PageSize == 0 {
		request.PageSize = LineagePageSize
	}
	if request.PageSize < 1 || request.PageSize > LineagePageSize {
		return CaptureRequest{}, fmt.Errorf("lineage page size must be between 1 and %d", LineagePageSize)
	}
	return request, nil
}

// StaticLineageRootQuery returns the frozen identity-mapping query for a root.
// It performs no transport work and intentionally does not invent lineage edges.
func StaticLineageRootQuery(kind ResourceKind) (string, error) {
	switch kind {
	case KindWorkbook:
		return workbookLineageRootQuery, nil
	case KindPublishedDatasource:
		return publishedDatasourceLineageRootQuery, nil
	case KindFlow:
		return flowLineageRootQuery, nil
	default:
		return "", fmt.Errorf("unsupported lineage root kind %q", kind)
	}
}

const workbookLineageRootQuery = `query LineageWorkbookRoot($rootLuid: String!, $after: String, $pageSize: Int!) {
  workbooksConnection(first: $pageSize, after: $after, filter: {luid: $rootLuid}, permissionMode: OBFUSCATE_RESULTS) {
    totalCount
    pageInfo { hasNextPage endCursor }
    nodes { id luid name }
  }
}`

const publishedDatasourceLineageRootQuery = `query LineagePublishedDatasourceRoot($rootLuid: String!, $after: String, $pageSize: Int!) {
  publishedDatasourcesConnection(first: $pageSize, after: $after, filter: {luid: $rootLuid}, permissionMode: OBFUSCATE_RESULTS) {
    totalCount
    pageInfo { hasNextPage endCursor }
    nodes { id luid name }
  }
}`

const flowLineageRootQuery = `query LineageFlowRoot($rootLuid: String!, $after: String, $pageSize: Int!) {
  flowsConnection(first: $pageSize, after: $after, filter: {luid: $rootLuid}, permissionMode: OBFUSCATE_RESULTS) {
    totalCount
    pageInfo { hasNextPage endCursor }
    nodes { id luid name }
  }
}`

func lineageRelationQuery(kind ResourceKind, relation lineageRelation) (string, error) {
	rootField, operation, err := lineageRootField(kind)
	if err != nil {
		return "", err
	}
	nodes := "nodes { id luid name }"
	if relation.linked {
		nodes = "nodes { asset { id luid name } fromEdges toEdges }"
	}
	return fmt.Sprintf(`query %s($rootLuid: String!, $after: String, $pageSize: Int!) {
  %s(first: 2, filter: {luid: $rootLuid}, permissionMode: OBFUSCATE_RESULTS) {
    totalCount
    pageInfo { hasNextPage endCursor }
    nodes {
      id
      luid
      name
      %s(first: $pageSize, after: $after, orderBy: {field: ID, direction: ASC}, permissionMode: OBFUSCATE_RESULTS) {
        totalCount
        pageInfo { hasNextPage endCursor }
        %s
      }
    }
  }
}`, operation+lineageOperationSuffix(relation.field), rootField, relation.field, nodes), nil
}

func lineageRootField(kind ResourceKind) (string, string, error) {
	switch kind {
	case KindWorkbook:
		return "workbooksConnection", "LineageWorkbook", nil
	case KindPublishedDatasource:
		return "publishedDatasourcesConnection", "LineagePublishedDatasource", nil
	case KindFlow:
		return "flowsConnection", "LineageFlow", nil
	default:
		return "", "", fmt.Errorf("unsupported lineage root kind %q", kind)
	}
}

func lineageOperationSuffix(field string) string {
	field = strings.TrimSuffix(field, "Connection")
	if field == "" {
		return "Relation"
	}
	return strings.ToUpper(field[:1]) + field[1:]
}

type lineageEnvelope struct {
	Data     lineageData     `json:"data"`
	Errors   []graphqlIssue  `json:"errors"`
	Warnings json.RawMessage `json:"warnings"`
}

type lineageData struct {
	WorkbooksConnection            *lineageConnection `json:"workbooksConnection"`
	PublishedDatasourcesConnection *lineageConnection `json:"publishedDatasourcesConnection"`
	FlowsConnection                *lineageConnection `json:"flowsConnection"`
}

func (d lineageData) connection(kind ResourceKind) *lineageConnection {
	switch kind {
	case KindWorkbook:
		return d.WorkbooksConnection
	case KindPublishedDatasource:
		return d.PublishedDatasourcesConnection
	case KindFlow:
		return d.FlowsConnection
	default:
		return nil
	}
}

type lineageConnection struct {
	TotalCount *int           `json:"totalCount"`
	PageInfo   *pageInfo      `json:"pageInfo"`
	Nodes      *[]lineageNode `json:"nodes"`
}

type lineageNode struct {
	ID                              string             `json:"id"`
	LUID                            string             `json:"luid"`
	Name                            string             `json:"name"`
	Asset                           *lineageNode       `json:"asset"`
	UpstreamDatasourcesConnection   *lineageConnection `json:"upstreamDatasourcesConnection"`
	UpstreamFlowsConnection         *lineageConnection `json:"upstreamFlowsConnection"`
	UpstreamLinkedFlowsConnection   *lineageConnection `json:"upstreamLinkedFlowsConnection"`
	DownstreamDatasourcesConnection *lineageConnection `json:"downstreamDatasourcesConnection"`
	DownstreamFlowsConnection       *lineageConnection `json:"downstreamFlowsConnection"`
	DownstreamLinkedFlowsConnection *lineageConnection `json:"downstreamLinkedFlowsConnection"`
	DownstreamWorkbooksConnection   *lineageConnection `json:"downstreamWorkbooksConnection"`
}

func (n lineageNode) relation(relation lineageRelation) *lineageConnection {
	switch relation.field {
	case "upstreamDatasourcesConnection":
		return n.UpstreamDatasourcesConnection
	case "upstreamFlowsConnection":
		return n.UpstreamFlowsConnection
	case "upstreamLinkedFlowsConnection":
		return n.UpstreamLinkedFlowsConnection
	case "downstreamDatasourcesConnection":
		return n.DownstreamDatasourcesConnection
	case "downstreamFlowsConnection":
		return n.DownstreamFlowsConnection
	case "downstreamLinkedFlowsConnection":
		return n.DownstreamLinkedFlowsConnection
	case "downstreamWorkbooksConnection":
		return n.DownstreamWorkbooksConnection
	default:
		return nil
	}
}
