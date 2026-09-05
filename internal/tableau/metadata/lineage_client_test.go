package metadata

import (
	"context"
	"encoding/json"
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
	if fmt.Sprint(capture.RequestIDs) != "[request-1 request-2 request-3]" {
		t.Fatalf("request IDs = %#v", capture.RequestIDs)
	}
	gotRequests := *requests
	if len(gotRequests) != 3 || !strings.Contains(gotRequests[1].Query, "upstreamDatasourcesConnection") {
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
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamLinkedFlowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"asset":{"id":"downstream-meta","luid":"downstream-rest","name":"After"},"fromEdges":["step-2"],"toEdges":[]}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamWorkbooksConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
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

func TestCaptureLineageDropsSelfReferentialFlowEdges(t *testing.T) {
	responses := []string{
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current"}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","upstreamLinkedFlowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"asset":{"id":"flow-meta","luid":"flow-rest","name":"Current"},"fromEdges":[],"toEdges":["step-1"]}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamLinkedFlowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"asset":{"id":"flow-meta","luid":"flow-rest","name":"Current"},"fromEdges":["step-2"],"toEdges":[]}]}}]}}}`,
		`{"data":{"flowsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"flow-meta","luid":"flow-rest","name":"Current","downstreamWorkbooksConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
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
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-b-meta","luid":"ds-b","name":"B","upstreamDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-a-meta","luid":"ds-a","name":"A"}]}}]}}}`,
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-b-meta","luid":"ds-b","name":"B","upstreamFlowsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`,
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

func TestCaptureLineageCarriesEarlierPageWarningThroughConnectionCompletion(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book"}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":3,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"datasource-a","luid":"rest-a","name":"A"}]}}]}},"warnings":[{"code":"NODE_LIMIT_EXCEEDED"}]}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"workbook-meta","luid":"workbook-rest","name":"Book","upstreamDatasourcesConnection":{"totalCount":3,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"datasource-b","luid":"rest-b","name":"B"}]}}]}}}`,
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
