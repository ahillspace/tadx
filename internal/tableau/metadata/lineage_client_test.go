package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
)

func TestCaptureLineageMapsWorkbookRESTIdentityAndPagesDirectUpstreamDatasources(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"id":"metadata-workbook","luid":"workbook-rest","name":"Book"}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"id":"metadata-workbook","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"datasource-page-1"},"nodes":[{"id":"metadata-datasource-b","luid":"datasource-rest-b","name":"Beta"}]}}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"id":"metadata-workbook","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":"datasource-page-2"},"nodes":[{"id":"metadata-datasource-a","luid":"datasource-rest-a","name":"Alpha"}]}}]}}}`,
		emptyLineageRelationResponse(t, "workbooksConnection", "metadata-workbook", "workbook-rest", "Book", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "workbooksConnection", "metadata-workbook", "workbook-rest", "Book", "upstreamTablesConnection"),
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest", Direction: DirectionUpstream, Depth: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if capture.RootRESTLUID != "workbook-rest" || capture.RootMetadataID != "metadata-workbook" || !capture.Complete {
		t.Fatalf("capture identity = %#v", capture)
	}
	wantNodes := []Node{
		{MetadataID: "metadata-datasource-a", Kind: "published_datasource", RESTLUID: "datasource-rest-a", Name: "Alpha"},
		{MetadataID: "metadata-datasource-b", Kind: "published_datasource", RESTLUID: "datasource-rest-b", Name: "Beta"},
		{MetadataID: "metadata-workbook", Kind: "workbook", RESTLUID: "workbook-rest", Name: "Book"},
	}
	if fmt.Sprint(capture.Nodes) != fmt.Sprint(wantNodes) {
		t.Fatalf("nodes = %#v, want %#v", capture.Nodes, wantNodes)
	}
	wantEdges := []Edge{
		{FromMetadataID: "metadata-datasource-a", ToMetadataID: "metadata-workbook", Relationship: "upstream"},
		{FromMetadataID: "metadata-datasource-b", ToMetadataID: "metadata-workbook", Relationship: "upstream"},
	}
	if fmt.Sprint(capture.Edges) != fmt.Sprint(wantEdges) {
		t.Fatalf("edges = %#v, want %#v", capture.Edges, wantEdges)
	}
	if fmt.Sprint(capture.RequestIDs) != "[request-1 request-2 request-3 request-4 request-5]" {
		t.Fatalf("request IDs = %#v", capture.RequestIDs)
	}
	gotRequests := *requests
	if len(gotRequests) != 5 || !strings.Contains(gotRequests[1].Query, "upstreamDatasourcesConnection") || !strings.Contains(gotRequests[3].Query, "upstreamDatabasesConnection") || !strings.Contains(gotRequests[4].Query, "upstreamTablesConnection") {
		t.Fatalf("requests = %#v", gotRequests)
	}
	if gotRequests[1].Variables["after"] != nil || gotRequests[2].Variables["after"] != "datasource-page-1" || gotRequests[1].Variables["pageSize"] != float64(1) {
		t.Fatalf("pagination variables = %#v then %#v", gotRequests[1].Variables, gotRequests[2].Variables)
	}
}

func TestCaptureLineageUsesLinkedFlowConnectionsForDirectFlowEdges(t *testing.T) {
	responses := []string{
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current"}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamLinkedFlowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"asset":{"id":"upstream-meta","luid":"upstream-rest","name":"Before"},"fromEdges":[],"toEdges":["step-1"]}]}}]}}}`,
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamTablesConnection"),
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamLinkedFlowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"asset":{"id":"downstream-meta","luid":"downstream-rest","name":"After"},"fromEdges":["step-2"],"toEdges":[]}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamWorkbooksConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "downstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "downstreamTablesConnection"),
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindFlow, RESTLUID: "flow-rest", Direction: DirectionBoth, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := []Edge{
		{FromMetadataID: "flow-meta", ToMetadataID: "downstream-meta", Relationship: "downstream"},
		{FromMetadataID: "upstream-meta", ToMetadataID: "flow-meta", Relationship: "upstream"},
	}
	if fmt.Sprint(capture.Edges) != fmt.Sprint(want) {
		t.Fatalf("edges = %#v, want %#v", capture.Edges, want)
	}
	queries := make([]string, 0, len(*requests))
	for _, request := range *requests {
		queries = append(queries, request.Query)
	}
	joined := strings.Join(queries, "\n")
	if !strings.Contains(joined, "upstreamLinkedFlowsConnection") || !strings.Contains(joined, "downstreamLinkedFlowsConnection") || strings.Contains(joined, "upstreamFlowsConnection") || strings.Contains(joined, "downstreamFlowsConnection") {
		t.Fatalf("flow queries did not use linked-flow structure:\n%s", joined)
	}
}

func TestCaptureLineageCapturesFlowPhysicalDatabasesAndTablesWithoutRESTIdentities(t *testing.T) {
	responses := []string{
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current"}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamLinkedFlowsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamDatabasesConnection":{"totalCount":6,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"database-6","name":"Six"},{"id":"database-5","name":"Five"},{"id":"database-4","name":"Four"},{"id":"database-3","name":"Three"},{"id":"database-2","name":"Two"},{"id":"database-1","name":"One"}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamTablesConnection":{"totalCount":6,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"table-6","name":"Six"},{"id":"table-5","name":"Five"},{"id":"table-4","name":"Four"},{"id":"table-3","name":"Three"},{"id":"table-2","name":"Two"},{"id":"table-1","name":"One"}]}}]}}}`,
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindFlow, RESTLUID: "flow-rest", Direction: DirectionUpstream, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !capture.Complete || len(capture.Nodes) != 13 || len(capture.Edges) != 12 {
		t.Fatalf("capture = %#v", capture)
	}
	for _, node := range capture.Nodes {
		if (node.Kind == string(KindDatabase) || node.Kind == string(KindTable)) && node.RESTLUID != "" {
			t.Fatalf("physical node has a REST identity: %#v", node)
		}
	}
	if capture.Nodes[0] != (Node{MetadataID: "database-1", Kind: "database", Name: "One"}) || capture.Nodes[6] != (Node{MetadataID: "flow-meta", Kind: "flow", RESTLUID: "flow-rest", Name: "Current"}) || capture.Nodes[7] != (Node{MetadataID: "table-1", Kind: "table", Name: "One"}) {
		t.Fatalf("deterministic nodes = %#v", capture.Nodes)
	}
	for _, edge := range capture.Edges {
		if edge.ToMetadataID != "flow-meta" || edge.Relationship != "upstream" {
			t.Fatalf("physical edge = %#v", edge)
		}
	}
	joined := ""
	for _, request := range *requests {
		joined += request.Query
	}
	if !strings.Contains(joined, "upstreamDatabasesConnection") || !strings.Contains(joined, "upstreamTablesConnection") {
		t.Fatalf("physical queries = %s", joined)
	}
}

func TestCaptureLineageExpandsTableParentDatabaseByMetadataIDAtDepthTwo(t *testing.T) {
	responses := []string{
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current"}]}}}`,
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamDatasourcesConnection"),
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamLinkedFlowsConnection"),
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamDatabasesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"database-meta","name":"Warehouse"}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamTablesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"table-meta","name":"Orders"}]}}]}}}`,
		`{"data":{"databaseTablesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"table-meta","name":"Orders","database":{"id":"database-meta","name":"Warehouse"}}]}}}`,
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindFlow, RESTLUID: "flow-rest", Direction: DirectionUpstream, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	wantEdges := []Edge{
		{FromMetadataID: "database-meta", ToMetadataID: "flow-meta", Relationship: "upstream"},
		{FromMetadataID: "database-meta", ToMetadataID: "table-meta", Relationship: "upstream"},
		{FromMetadataID: "table-meta", ToMetadataID: "flow-meta", Relationship: "upstream"},
	}
	if !capture.Complete || fmt.Sprint(capture.Edges) != fmt.Sprint(wantEdges) {
		t.Fatalf("capture = %#v, want edges %#v", capture, wantEdges)
	}
	last := (*requests)[len(*requests)-1]
	if !strings.Contains(last.Query, "databaseTablesConnection") || !strings.Contains(last.Query, "database { id name }") || last.Variables["rootMetadataID"] != "table-meta" || strings.Contains(last.Query, "rootLuid") {
		t.Fatalf("table expansion request = %#v", last)
	}
}

func TestCaptureLineageCapturesDownstreamPhysicalEdgesForFlow(t *testing.T) {
	responses := []string{
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current"}]}}}`,
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "downstreamDatasourcesConnection"),
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "downstreamLinkedFlowsConnection"),
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "downstreamWorkbooksConnection"),
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamDatabasesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"database-meta","name":"Output"}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamTablesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"table-meta","name":"Output"}]}}]}}}`,
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindFlow, RESTLUID: "flow-rest", Direction: DirectionDownstream, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := []Edge{
		{FromMetadataID: "flow-meta", ToMetadataID: "database-meta", Relationship: "downstream"},
		{FromMetadataID: "flow-meta", ToMetadataID: "table-meta", Relationship: "downstream"},
	}
	if !capture.Complete || fmt.Sprint(capture.Edges) != fmt.Sprint(want) {
		t.Fatalf("capture = %#v, want edges %#v", capture, want)
	}
	if len(*requests) != len(responses) {
		t.Fatalf("downstream-only depth traversal made %d requests, want %d", len(*requests), len(responses))
	}
}

func TestCaptureLineageDistinguishesNullAndOmittedTableDatabase(t *testing.T) {
	for _, test := range []struct {
		name         string
		databaseJSON string
		wantError    bool
	}{
		{name: "explicit null is no parent relationship", databaseJSON: `,"database":null`},
		{name: "omitted field is malformed", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			responses := []string{
				`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current"}]}}}`,
				emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamDatasourcesConnection"),
				emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamLinkedFlowsConnection"),
				emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamDatabasesConnection"),
				`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamTablesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"table-meta","name":"Orders"}]}}]}}}`,
				`{"data":{"databaseTablesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"table-meta","name":"Orders"` + test.databaseJSON + `}]}}}`,
			}
			server, _ := lineageTestServer(t, responses)
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindFlow, RESTLUID: "flow-rest", Direction: DirectionUpstream, Depth: 2})
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "database field was omitted") {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil || !capture.Complete || len(capture.Nodes) != 2 || len(capture.Edges) != 1 {
				t.Fatalf("capture = %#v, error = %v", capture, err)
			}
		})
	}
}

func TestCaptureLineageDropsSelfReferentialFlowEdges(t *testing.T) {
	responses := []string{
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current"}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamLinkedFlowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"asset":{"id":"flow-meta","luid":"flow-rest","name":"Current"},"fromEdges":[],"toEdges":["step-1"]}]}}]}}}`,
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "upstreamTablesConnection"),
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamLinkedFlowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"asset":{"id":"flow-meta","luid":"flow-rest","name":"Current"},"fromEdges":["step-2"],"toEdges":[]}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamWorkbooksConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "downstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "flowsConnection", "flow-meta", "flow-rest", "Current", "downstreamTablesConnection"),
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindFlow, RESTLUID: "flow-rest", Direction: DirectionBoth, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if capture.Complete || len(capture.Edges) != 0 || len(capture.Warnings) != 1 || !strings.Contains(capture.Warnings[0], "self-referential") {
		t.Fatalf("capture=%#v", capture)
	}
}

func TestCaptureLineageTraversesDepthWithoutInferringCycles(t *testing.T) {
	responses := []string{
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-a-meta","luid":"ds-a","name":"A"}]}}}`,
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-a-meta","luid":"ds-a","name":"A","upstreamDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-b-meta","luid":"ds-b","name":"B"}]}}]}}}`,
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-a-meta","luid":"ds-a","name":"A","upstreamFlowsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-a-meta", "ds-a", "A", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-a-meta", "ds-a", "A", "upstreamTablesConnection"),
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-b-meta","luid":"ds-b","name":"B","upstreamDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-a-meta","luid":"ds-a","name":"A"}]}}]}}}`,
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-b-meta","luid":"ds-b","name":"B","upstreamFlowsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-b-meta", "ds-b", "B", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-b-meta", "ds-b", "B", "upstreamTablesConnection"),
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindPublishedDatasource, RESTLUID: "ds-a", Direction: DirectionUpstream, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Nodes) != 2 || len(capture.Edges) != 2 {
		t.Fatalf("capture = %#v", capture)
	}
}

func TestCaptureLineageReturnsBoundedIncompleteCaptureForMetadataWarnings(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}},"warnings":[{"message":"partial data with session-secret","code":"NODE_LIMIT_EXCEEDED"}]}`,
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-secret", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest"})
	if err != nil {
		t.Fatal(err)
	}
	if capture.Complete || len(capture.Warnings) != 1 || strings.Contains(capture.Warnings[0], "session-secret") || !strings.Contains(capture.Warnings[0], "NODE_LIMIT_EXCEEDED") {
		t.Fatalf("capture = %#v", capture)
	}
}

func TestCaptureLineageRetainsPartialRelationshipDataWithoutClaimingCompleteness(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"datasource-meta","luid":"datasource-rest","name":"Sales"}]}}]}},"errors":[{"message":"partial nodes","extensions":{"code":"NODE_LIMIT_EXCEEDED","severity":"WARNING"}}]}`,
		emptyLineageRelationResponse(t, "workbooksConnection", "workbook-meta", "workbook-rest", "Book", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "workbooksConnection", "workbook-meta", "workbook-rest", "Book", "upstreamTablesConnection"),
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest", Direction: DirectionUpstream})
	if err != nil {
		t.Fatal(err)
	}
	if capture.Complete || len(capture.Nodes) != 2 || len(capture.Edges) != 1 || len(capture.Warnings) != 1 {
		t.Fatalf("capture = %#v", capture)
	}
}

func TestCaptureLineageReturnsValidatedPartialGraphWithRelationFailure(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"datasource-meta","luid":"datasource-rest","name":"Sales"}]}}]}}}`,
		`{"errors":[{"message":"permission denied","extensions":{"code":"ACCESS_DENIED"}}]}`,
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest", Direction: DirectionUpstream})
	if err == nil {
		t.Fatal("expected relation failure")
	}
	var protocol *tableau.ProtocolError
	if !errors.As(err, &protocol) || tableau.RequestID(err) != "request-3" {
		t.Fatalf("error = %v, request ID = %q", err, tableau.RequestID(err))
	}
	var relation *RelationError
	if !errors.As(err, &relation) || relation.Relation != "upstreamDatabasesConnection" || relation.RootRESTLUID != "workbook-rest" {
		t.Fatalf("relation error = %#v", relation)
	}
	if capture.Complete || len(capture.Nodes) != 2 || len(capture.Edges) != 1 {
		t.Fatalf("partial capture = %#v", capture)
	}
}

func TestCaptureLineageRetainsOnlyPriorPagesWhenCurrentPageIsMalformed(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"page-1"},"nodes":[{"id":"datasource-a","luid":"rest-a","name":"A"}]}}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":2,"pageInfo":{"endCursor":"page-2"},"nodes":[{"id":"datasource-b","luid":"rest-b","name":"B"}]}}]}}}`,
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest", Direction: DirectionUpstream})
	if err == nil || !strings.Contains(err.Error(), "hasNextPage") {
		t.Fatalf("error = %v", err)
	}
	if capture.Complete || len(capture.Nodes) != 2 || len(capture.Edges) != 1 {
		t.Fatalf("capture = %#v", capture)
	}
	for _, node := range capture.Nodes {
		if node.MetadataID == "datasource-b" {
			t.Fatalf("malformed current page was retained: %#v", capture.Nodes)
		}
	}
}

func TestCaptureLineageCarriesEarlierPageWarningThroughConnectionCompletion(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":3,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"datasource-a","luid":"rest-a","name":"A"}]}}]}},"warnings":[{"code":"NODE_LIMIT_EXCEEDED"}]}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":3,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"datasource-b","luid":"rest-b","name":"B"}]}}]}}}`,
		emptyLineageRelationResponse(t, "workbooksConnection", "workbook-meta", "workbook-rest", "Book", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "workbooksConnection", "workbook-meta", "workbook-rest", "Book", "upstreamTablesConnection"),
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest", Direction: DirectionUpstream})
	if err != nil {
		t.Fatal(err)
	}
	if capture.Complete || len(capture.Nodes) != 3 || len(capture.Warnings) != 1 {
		t.Fatalf("capture = %#v", capture)
	}
}

func TestCaptureLineageDoesNotPersistUntrustedWarningCodesOrMessages(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}},"warnings":[{"message":"session-secret","code":"session-secret","extensions":{"severity":"WARNING"}}]}`,
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-secret", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest"})
	if err != nil {
		t.Fatal(err)
	}
	if capture.Complete || len(capture.Warnings) != 1 || strings.Contains(strings.ToLower(capture.Warnings[0]), "session-secret") || !strings.Contains(capture.Warnings[0], "UNKNOWN_WARNING") {
		t.Fatalf("capture warnings = %#v", capture.Warnings)
	}
}

func TestCaptureLineageEnforcesTransportNodeBound(t *testing.T) {
	nodes := make([]map[string]string, MaxLineageNodes)
	for index := range nodes {
		nodes[index] = map[string]string{"id": fmt.Sprintf("metadata-%03d", index), "luid": fmt.Sprintf("rest-%03d", index), "name": fmt.Sprintf("Node %03d", index)}
	}
	page, err := json.Marshal(map[string]any{
		"data": map[string]any{"workbooksConnection": map[string]any{
			"totalCount": 1, "pageInfo": map[string]any{"hasNextPage": false},
			"nodes": []any{map[string]any{
				"id": "workbook-meta", "luid": "workbook-rest", "name": "Book",
				"upstreamDatasourcesConnection": map[string]any{"totalCount": len(nodes), "pageInfo": map[string]any{"hasNextPage": false}, "nodes": nodes},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}}}`,
		string(page),
	}
	server, _ := lineageTestServer(t, responses)
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest", Direction: DirectionUpstream})
	if err != nil {
		t.Fatal(err)
	}
	if capture.Complete || len(capture.Nodes) != MaxLineageNodes || len(capture.Edges) != MaxLineageNodes-1 || !strings.Contains(strings.Join(capture.Warnings, " "), "500-node") {
		t.Fatalf("node count = %d, edge count = %d, complete = %v, warnings = %#v", len(capture.Nodes), len(capture.Edges), capture.Complete, capture.Warnings)
	}
}

func TestCaptureLineageRejectsFatalGraphQLIssuesWithRequestContext(t *testing.T) {
	server, _ := lineageTestServer(t, []string{`{"errors":[{"message":"denied","extensions":{"code":"ACCESS_DENIED"}}]}`})
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	_, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest"})
	if err == nil || !strings.Contains(err.Error(), "ACCESS_DENIED") || tableau.RequestID(err) != "request-1" {
		t.Fatalf("error = %v, request ID = %q", err, tableau.RequestID(err))
	}
}

func TestCaptureLineageRejectsMalformedConnectionsAndRepeatedCursors(t *testing.T) {
	tests := []struct {
		name      string
		responses []string
		want      string
	}{
		{
			name:      "root identity mismatch",
			responses: []string{`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"other","name":"Book"}]}}}`},
			want:      "expected REST LUID",
		},
		{
			name: "missing connection fields",
			responses: []string{
				`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"workbook-rest","name":"Book"}]}}}`,
				`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":0,"nodes":[]}}]}}}`,
			},
			want: "pageInfo",
		},
		{
			name: "repeated cursor",
			responses: []string{
				`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"workbook-rest","name":"Book"}]}}}`,
				`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"again"},"nodes":[{"id":"a","luid":"a","name":"A"}]}}]}}}`,
				`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"again"},"nodes":[{"id":"b","luid":"b","name":"B"}]}}]}}}`,
			},
			want: "repeated cursor",
		},
		{
			name: "root label changed while paging",
			responses: []string{
				`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"workbook-rest","name":"Book"}]}}}`,
				`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"workbook-rest","name":"Changed","upstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
			},
			want: "changed identity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, _ := lineageTestServer(t, test.responses)
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			_, err := client.CaptureLineage(context.Background(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "workbook-rest", Direction: DirectionUpstream})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func lineageTestServer(t *testing.T, responses []string) (*httptest.Server, *[]capturedGraphQLRequest) {
	t.Helper()
	requests := make([]capturedGraphQLRequest, 0, len(responses))
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.EscapedPath() != "/api/metadata/graphql" {
			t.Errorf("request = %s %s", request.Method, request.URL.EscapedPath())
		}
		var payload capturedGraphQLRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		index := len(requests)
		requests = append(requests, payload)
		if index >= len(responses) {
			t.Errorf("unexpected request %d: %s", index+1, payload.Query)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Tableau-Request-Id", fmt.Sprintf("request-%d", index+1))
		_, _ = io.WriteString(writer, responses[index])
	}))
	return server, &requests
}

func emptyLineageRelationResponse(t *testing.T, rootField, metadataID, restLUID, name, relationField string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"data": map[string]any{
			rootField: map[string]any{
				"totalCount": 1,
				"pageInfo":   map[string]any{"hasNextPage": false},
				"nodes": []any{map[string]any{
					"id": metadataID, "luid": restLUID, "name": name,
					relationField: map[string]any{"totalCount": 0, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}},
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
