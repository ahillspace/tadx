package metadata

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau"
)

// Field enrichment uses the captured asset graph as its scope and capacity.
const lineageFieldSelection = `fieldsConnection(first: $pageSize, after: $after, orderBy: {field: ID, direction: ASC}, permissionMode: OBFUSCATE_RESULTS) {
  totalCount pageInfo { hasNextPage endCursor }
  nodes {
    __typename id name datasource { id }
    upstreamColumnsConnection(first: 100, orderBy: {field: ID, direction: ASC}, permissionMode: OBFUSCATE_RESULTS) {
      totalCount pageInfo { hasNextPage endCursor }
      nodes { id name table { id } }
    }
    ... on DatasourceField { remoteField { id name datasource { id } } }
    ... on CalculatedField {
      fieldsConnection(first: 100, orderBy: {field: ID, direction: ASC}, permissionMode: OBFUSCATE_RESULTS) {
        totalCount pageInfo { hasNextPage endCursor }
        nodes { id name datasource { id } }
      }
    }
  }
}`

func lineageFieldsQuery(owner Node) (string, map[string]any) {
	rootField := "publishedDatasourcesConnection"
	rootSelector := "luid: $rootLuid"
	rootVariable := "$rootLuid: String!"
	rootIdentity := "id luid name"
	variables := map[string]any{"rootLuid": owner.RESTLUID, "after": nil, "pageSize": LineagePageSize}
	if owner.Kind == string(KindEmbeddedDatasource) {
		rootField = "embeddedDatasourcesConnection"
		rootSelector = "id: $rootMetadataID"
		rootVariable = "$rootMetadataID: ID!"
		rootIdentity = "id name"
		variables = map[string]any{"rootMetadataID": owner.MetadataID, "after": nil, "pageSize": LineagePageSize}
	}
	query := fmt.Sprintf(`query LineageDatasourceFields(%s, $after: String, $pageSize: Int!) {
  %s(first: 2, filter: {%s}, permissionMode: OBFUSCATE_RESULTS) {
    totalCount pageInfo { hasNextPage endCursor }
    nodes { %s %s }
  }
}`, rootVariable, rootField, rootSelector, rootIdentity, lineageFieldSelection)
	return query, variables
}

func (c *Client) enrichLineageFields(ctx context.Context, request CaptureRequest, capture *Capture, nodes map[string]Node, restIdentities map[string]string, edges map[string]Edge) error {
	owners := make([]Node, 0)
	for _, node := range nodes {
		if node.Kind == string(KindPublishedDatasource) || node.Kind == string(KindEmbeddedDatasource) {
			owners = append(owners, node)
		}
	}
	slices.SortFunc(owners, func(a, b Node) int { return strings.Compare(a.MetadataID, b.MetadataID) })
	assetIDs := make(map[string]struct{}, len(nodes))
	for id := range nodes {
		assetIDs[id] = struct{}{}
	}
	for _, owner := range owners {
		if len(nodes) >= MaxLineageNodes || len(edges) >= MaxLineageEdges {
			markLineageFieldBound(capture, len(nodes) >= MaxLineageNodes)
			return nil
		}
		if err := c.readLineageFields(ctx, request, owner, capture, nodes, restIdentities, edges, assetIDs); err != nil {
			if _, ok := errors.AsType[*RelationError](err); ok {
				return err
			}
			return &RelationError{RootKind: request.Kind, RootRESTLUID: request.RESTLUID, Relation: "fieldsConnection", Cause: err}
		}
	}
	return nil
}

func (c *Client) readLineageFields(ctx context.Context, request CaptureRequest, owner Node, capture *Capture, nodes map[string]Node, restIdentities map[string]string, edges map[string]Edge, assetIDs map[string]struct{}) error {
	query, variables := lineageFieldsQuery(owner)
	variables["pageSize"] = request.PageSize
	seenCursors := make(map[string]struct{})
	seenFields := make(map[string]struct{})
	total := -1
	for {
		response, envelope, warnings, err := c.queryLineage(ctx, query, variables)
		capture.RequestIDs = append(capture.RequestIDs, response.TableauRequestID)
		capture.Warnings = append(capture.Warnings, warnings...)
		if len(warnings) > 0 {
			capture.Complete = false
		}
		if err != nil {
			capture.RequestIDs = append(capture.RequestIDs, tableau.RequestID(err))
			capture.Warnings = append(capture.Warnings, "Lineage fieldsConnection could not be captured; the graph is incomplete.")
			return err
		}
		connection := envelope.Data.connection(ResourceKind(owner.Kind))
		var returned Node
		if owner.Kind == string(KindEmbeddedDatasource) {
			returned, err = exactMetadataLineageRoot(response, connection, KindEmbeddedDatasource, owner.MetadataID)
		} else {
			returned, err = exactLineageRoot(response, connection, KindPublishedDatasource, owner.RESTLUID)
		}
		if err != nil {
			return err
		}
		if returned != owner {
			return protocolError(response, "field owner %q changed identity while paging", owner.MetadataID)
		}
		fields := (*connection.Nodes)[0].FieldsConnection
		if fields == nil || fields.Nodes == nil || fields.TotalCount == nil || fields.PageInfo == nil || fields.PageInfo.HasNextPage == nil {
			return protocolError(response, "fieldsConnection omitted totalCount, pageInfo, or nodes")
		}
		if *fields.TotalCount < 0 {
			return protocolError(response, "fieldsConnection returned a negative totalCount")
		}
		if total < 0 {
			total = *fields.TotalCount
		} else if total != *fields.TotalCount {
			return protocolError(response, "fieldsConnection changed totalCount while paging")
		}
		for _, raw := range *fields.Nodes {
			id := strings.TrimSpace(raw.ID)
			if id == "" || raw.Datasource == nil || strings.TrimSpace(raw.Datasource.ID) != owner.MetadataID {
				return protocolError(response, "fieldsConnection returned a field without its owner identity")
			}
			if _, exists := seenFields[id]; exists {
				return protocolError(response, "fieldsConnection repeated Metadata ID %q", id)
			}
			seenFields[id] = struct{}{}
			field := Node{MetadataID: id, Kind: "field", Name: strings.TrimSpace(raw.Name)}
			if !addLineageFieldEvidence(field, Edge{FromMetadataID: owner.MetadataID, ToMetadataID: id, Relationship: "contains"}, nodes, restIdentities, edges, capture) {
				return nil
			}
			if err := c.addLineageFieldRelationships(ctx, request, response, owner, raw, field, assetIDs, nodes, restIdentities, edges, capture); err != nil {
				return err
			}
			if len(nodes) >= MaxLineageNodes || len(edges) >= MaxLineageEdges {
				markLineageFieldBound(capture, len(nodes) >= MaxLineageNodes)
				return nil
			}
		}
		if !*fields.PageInfo.HasNextPage {
			if len(seenFields) != total && len(warnings) == 0 {
				return protocolError(response, "fieldsConnection ended at %d of %d fields", len(seenFields), total)
			}
			return nil
		}
		cursor := strings.TrimSpace(fields.PageInfo.EndCursor)
		if cursor == "" {
			return protocolError(response, "fieldsConnection has another page without a cursor")
		}
		if _, exists := seenCursors[cursor]; exists {
			return protocolError(response, "fieldsConnection repeated cursor %q", cursor)
		}
		seenCursors[cursor] = struct{}{}
		variables["after"] = cursor
	}
}

func (c *Client) addLineageFieldRelationships(ctx context.Context, request CaptureRequest, response tableau.Response, owner Node, raw lineageNode, field Node, assetIDs map[string]struct{}, nodes map[string]Node, restIdentities map[string]string, edges map[string]Edge, capture *Capture) error {
	if raw.RemoteField != nil {
		if err := addScopedFieldLink(response, *raw.RemoteField, field, "remote_field", assetIDs, nodes, restIdentities, edges, capture); err != nil {
			return err
		}
	}
	if raw.Typename == "CalculatedField" {
		if raw.FieldsConnection == nil {
			return protocolError(response, "calculated field %q omitted fieldsConnection", field.MetadataID)
		}
		if err := c.walkNestedLineageFields(ctx, request, owner, field, "fieldsConnection", response, raw.FieldsConnection, capture, nodes, edges, func(page tableau.Response, referenced lineageNode) error {
			return addScopedFieldLink(page, referenced, field, "references", assetIDs, nodes, restIdentities, edges, capture)
		}); err != nil {
			return err
		}
	}
	if raw.UpstreamColumnsConnection == nil {
		return protocolError(response, "field %q omitted upstreamColumnsConnection", field.MetadataID)
	}
	return c.walkNestedLineageFields(ctx, request, owner, field, "upstreamColumnsConnection", response, raw.UpstreamColumnsConnection, capture, nodes, edges, func(page tableau.Response, column lineageNode) error {
		if column.Table == nil || strings.TrimSpace(column.ID) == "" {
			return protocolError(page, "upstream column omitted its Metadata or table identity")
		}
		tableID := strings.TrimSpace(column.Table.ID)
		if _, scoped := assetIDs[tableID]; !scoped {
			return nil
		}
		columnNode := Node{MetadataID: strings.TrimSpace(column.ID), Kind: "column", Name: strings.TrimSpace(column.Name)}
		if !addLineageFieldEvidence(columnNode, Edge{FromMetadataID: columnNode.MetadataID, ToMetadataID: field.MetadataID, Relationship: "upstream"}, nodes, restIdentities, edges, capture) {
			return nil
		}
		addLineageFieldEvidence(columnNode, Edge{FromMetadataID: tableID, ToMetadataID: columnNode.MetadataID, Relationship: "contains"}, nodes, restIdentities, edges, capture)
		return nil
	})
}

func addScopedFieldLink(response tableau.Response, raw lineageNode, target Node, relationship string, assetIDs map[string]struct{}, nodes map[string]Node, restIdentities map[string]string, edges map[string]Edge, capture *Capture) error {
	if strings.TrimSpace(raw.ID) == "" || raw.Datasource == nil || strings.TrimSpace(raw.Datasource.ID) == "" {
		return protocolError(response, "%s link omitted field or datasource Metadata identity", relationship)
	}
	ownerID := strings.TrimSpace(raw.Datasource.ID)
	if _, scoped := assetIDs[ownerID]; !scoped {
		return nil
	}
	source := Node{MetadataID: strings.TrimSpace(raw.ID), Kind: "field", Name: strings.TrimSpace(raw.Name)}
	if !addLineageFieldEvidence(source, Edge{FromMetadataID: ownerID, ToMetadataID: source.MetadataID, Relationship: "contains"}, nodes, restIdentities, edges, capture) {
		return nil
	}
	addLineageFieldEvidence(source, Edge{FromMetadataID: source.MetadataID, ToMetadataID: target.MetadataID, Relationship: relationship}, nodes, restIdentities, edges, capture)
	return nil
}

func (c *Client) walkNestedLineageFields(ctx context.Context, request CaptureRequest, owner, field Node, relation string, response tableau.Response, connection *lineageConnection, capture *Capture, nodes map[string]Node, edges map[string]Edge, visit func(tableau.Response, lineageNode) error) error {
	total := -1
	seenNodes := make(map[string]struct{})
	seenCursors := make(map[string]struct{})
	for {
		if connection == nil || connection.TotalCount == nil || connection.Nodes == nil || connection.PageInfo == nil || connection.PageInfo.HasNextPage == nil {
			return protocolError(response, "%s omitted totalCount, pageInfo, or nodes", relation)
		}
		if *connection.TotalCount < 0 {
			return protocolError(response, "%s returned negative totalCount", relation)
		}
		if total < 0 {
			total = *connection.TotalCount
		} else if total != *connection.TotalCount {
			return protocolError(response, "%s changed totalCount while paging", relation)
		}
		for _, raw := range *connection.Nodes {
			id := strings.TrimSpace(raw.ID)
			if id == "" {
				return protocolError(response, "%s returned a node without Metadata identity", relation)
			}
			if _, exists := seenNodes[id]; exists {
				return protocolError(response, "%s repeated Metadata ID %q", relation, id)
			}
			seenNodes[id] = struct{}{}
			if len(seenNodes) > total {
				return protocolError(response, "%s returned more nodes than totalCount", relation)
			}
			if err := visit(response, raw); err != nil {
				return err
			}
			if len(nodes) >= MaxLineageNodes || len(edges) >= MaxLineageEdges {
				markLineageFieldBound(capture, len(nodes) >= MaxLineageNodes)
				return nil
			}
		}
		if !*connection.PageInfo.HasNextPage {
			if len(seenNodes) != total {
				return protocolError(response, "%s ended at %d of %d nodes", relation, len(seenNodes), total)
			}
			return nil
		}
		cursor := strings.TrimSpace(connection.PageInfo.EndCursor)
		if cursor == "" {
			return protocolError(response, "%s has another page without an end cursor", relation)
		}
		if _, exists := seenCursors[cursor]; exists {
			return protocolError(response, "%s repeated cursor %q", relation, cursor)
		}
		seenCursors[cursor] = struct{}{}
		remaining := MaxLineageEdges - len(edges)
		if remaining < 1 {
			markLineageFieldBound(capture, false)
			return nil
		}
		query, variables := lineageNestedQuery(owner, relation)
		variables["fieldID"] = field.MetadataID
		variables["after"] = cursor
		variables["pageSize"] = min(request.PageSize, remaining)
		var envelope lineageEnvelope
		var warnings []string
		var err error
		response, envelope, warnings, err = c.queryLineage(ctx, query, variables)
		capture.RequestIDs = append(capture.RequestIDs, response.TableauRequestID)
		capture.Warnings = append(capture.Warnings, warnings...)
		if len(warnings) > 0 {
			capture.Complete = false
		}
		if err != nil {
			capture.RequestIDs = append(capture.RequestIDs, tableau.RequestID(err))
			capture.Warnings = append(capture.Warnings, fmt.Sprintf("Lineage %s could not be captured; the graph is incomplete.", relation))
			return &RelationError{RootKind: request.Kind, RootRESTLUID: request.RESTLUID, Relation: relation, Cause: err}
		}
		root := envelope.Data.connection(ResourceKind(owner.Kind))
		var returned Node
		if owner.Kind == string(KindEmbeddedDatasource) {
			returned, err = exactMetadataLineageRoot(response, root, KindEmbeddedDatasource, owner.MetadataID)
		} else {
			returned, err = exactLineageRoot(response, root, KindPublishedDatasource, owner.RESTLUID)
		}
		if err != nil {
			return &RelationError{RootKind: request.Kind, RootRESTLUID: request.RESTLUID, Relation: relation, Cause: err}
		}
		if returned != owner {
			return protocolError(response, "field owner %q changed identity while paging %s", owner.MetadataID, relation)
		}
		parentFields := (*root.Nodes)[0].FieldsConnection
		if parentFields == nil || parentFields.TotalCount == nil || parentFields.Nodes == nil || parentFields.PageInfo == nil || parentFields.PageInfo.HasNextPage == nil || *parentFields.TotalCount != 1 || len(*parentFields.Nodes) != 1 || *parentFields.PageInfo.HasNextPage {
			return protocolError(response, "expected one field %q while paging %s", field.MetadataID, relation)
		}
		returnedField := (*parentFields.Nodes)[0]
		if strings.TrimSpace(returnedField.ID) != field.MetadataID || strings.TrimSpace(returnedField.Name) != field.Name || returnedField.Datasource == nil || strings.TrimSpace(returnedField.Datasource.ID) != owner.MetadataID {
			return protocolError(response, "field %q changed identity while paging %s", field.MetadataID, relation)
		}
		if relation == "fieldsConnection" {
			connection = returnedField.FieldsConnection
		} else {
			connection = returnedField.UpstreamColumnsConnection
		}
	}
}

func lineageNestedQuery(owner Node, relation string) (string, map[string]any) {
	rootField, rootSelector, rootVariable, rootIdentity := "publishedDatasourcesConnection", "luid: $rootLuid", "$rootLuid: String!", "id luid name"
	variables := map[string]any{"rootLuid": owner.RESTLUID}
	if owner.Kind == string(KindEmbeddedDatasource) {
		rootField, rootSelector, rootVariable, rootIdentity = "embeddedDatasourcesConnection", "id: $rootMetadataID", "$rootMetadataID: ID!", "id name"
		variables = map[string]any{"rootMetadataID": owner.MetadataID}
	}
	nested := `upstreamColumnsConnection(first: $pageSize, after: $after, orderBy: {field: ID, direction: ASC}, permissionMode: OBFUSCATE_RESULTS) { totalCount pageInfo { hasNextPage endCursor } nodes { id name table { id } } }`
	if relation == "fieldsConnection" {
		nested = `... on CalculatedField { fieldsConnection(first: $pageSize, after: $after, orderBy: {field: ID, direction: ASC}, permissionMode: OBFUSCATE_RESULTS) { totalCount pageInfo { hasNextPage endCursor } nodes { id name datasource { id } } } }`
	}
	query := fmt.Sprintf(`query LineageNestedFields(%s, $fieldID: ID!, $after: String, $pageSize: Int!) {
  %s(first: 2, filter: {%s}, permissionMode: OBFUSCATE_RESULTS) {
    totalCount pageInfo { hasNextPage endCursor }
    nodes { %s fieldsConnection(first: 2, filter: {id: $fieldID}, permissionMode: OBFUSCATE_RESULTS) {
      totalCount pageInfo { hasNextPage endCursor }
      nodes { id name datasource { id } %s }
    } }
  }
}`, rootVariable, rootField, rootSelector, rootIdentity, nested)
	return query, variables
}

func addLineageFieldEvidence(node Node, edge Edge, nodes map[string]Node, restIdentities map[string]string, edges map[string]Edge, capture *Capture) bool {
	if current, exists := nodes[node.MetadataID]; exists && current != node {
		capture.Complete = false
		capture.Warnings = append(capture.Warnings, "A field identity conflicted with another lineage node; field lineage is incomplete.")
		return false
	}
	if _, exists := nodes[node.MetadataID]; !exists && len(nodes) >= MaxLineageNodes {
		markLineageFieldBound(capture, true)
		return false
	}
	if edge.FromMetadataID == edge.ToMetadataID {
		capture.Complete = false
		capture.Warnings = append(capture.Warnings, "A self-referential field edge was omitted; field lineage is incomplete.")
		return false
	}
	if _, exists := edges[lineageEdgeKey(edge)]; !exists && len(edges) >= MaxLineageEdges {
		markLineageFieldBound(capture, false)
		return false
	}
	if err := addLineageNode(nodes, restIdentities, node); err != nil {
		capture.Complete = false
		capture.Warnings = append(capture.Warnings, "A field identity conflicted with another lineage node; field lineage is incomplete.")
		return false
	}
	edges[lineageEdgeKey(edge)] = edge
	return true
}

func markLineageFieldBound(capture *Capture, nodeBound bool) {
	capture.Complete = false
	if nodeBound {
		capture.Warnings = append(capture.Warnings, lineageNodeBoundWarning)
	} else {
		capture.Warnings = append(capture.Warnings, lineageEdgeBoundWarning)
	}
}
