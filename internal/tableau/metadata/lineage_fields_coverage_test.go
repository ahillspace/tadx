package metadata

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
)

func TestCaptureLineageRetainsOwnerPageEvidenceAfterNestedFailure(t *testing.T) {
	for _, relation := range []string{"fieldsConnection", "upstreamColumnsConnection"} {
		t.Run(relation, func(t *testing.T) {
			responses := fieldPagingAssets(t)
			responses[4] = `{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-meta","luid":"ds-rest","name":"Sales","upstreamTablesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"table-meta","name":"Orders"}]}}]}}}`
			connection := func(name, node string) string {
				more := name == relation
				total := 1
				if more {
					total++
				}
				return fmt.Sprintf(`{"totalCount":%d,"pageInfo":{"hasNextPage":%t,"endCursor":"next"},"nodes":[%s]}`, total, more, node)
			}
			fields := fmt.Sprintf(`[
				{"__typename":"CalculatedField","id":"calc-meta","name":"Margin","datasource":{"id":"ds-meta"},"fieldsConnection":%s,"upstreamColumnsConnection":%s},
				{"__typename":"DatasourceField","id":"sibling-meta","name":"Sibling","datasource":{"id":"ds-meta"},"remoteField":{"id":"base-meta","name":"Base","datasource":{"id":"ds-meta"}},"upstreamColumnsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"column-b","name":"B","table":{"id":"table-meta"}}]}}
			]`, connection("fieldsConnection", `{"id":"base-meta","name":"Base","datasource":{"id":"ds-meta"}}`), connection("upstreamColumnsConnection", `{"id":"column-a","name":"A","table":{"id":"table-meta"}}`))
			responses = append(responses, fieldPagingOwnerResponse(fields, 2, false, ""), `{"errors":[{"message":"nested page denied","extensions":{"code":"ACCESS_DENIED"}}]}`)
			server, requests := lineageTestServer(t, responses)
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			request := CaptureRequest{Kind: KindPublishedDatasource, RESTLUID: "ds-rest", Direction: DirectionUpstream, Fields: true}
			capture, err := client.CaptureLineage(t.Context(), request)
			failure := FailureFor(request, err)
			if err == nil || capture.Complete || failure.Relation != relation || failure.RequestID != "request-7" || len(*requests) != 7 {
				t.Fatalf("requests=%d, capture=%+v, failure=%+v, err=%v", len(*requests), capture, failure, err)
			}
			for _, node := range []Node{
				{MetadataID: "calc-meta", Kind: "field", Name: "Margin"},
				{MetadataID: "sibling-meta", Kind: "field", Name: "Sibling"},
				{MetadataID: "base-meta", Kind: "field", Name: "Base"},
				{MetadataID: "column-a", Kind: "column", Name: "A"},
				{MetadataID: "column-b", Kind: "column", Name: "B"},
			} {
				if !slices.Contains(capture.Nodes, node) {
					t.Errorf("confirmed node %+v was lost: %+v", node, capture.Nodes)
				}
			}
			for _, edge := range []Edge{
				{FromMetadataID: "ds-meta", ToMetadataID: "sibling-meta", Relationship: "contains"},
				{FromMetadataID: "base-meta", ToMetadataID: "calc-meta", Relationship: "references"},
				{FromMetadataID: "base-meta", ToMetadataID: "sibling-meta", Relationship: "remote_field"},
				{FromMetadataID: "column-a", ToMetadataID: "calc-meta", Relationship: "upstream"},
				{FromMetadataID: "table-meta", ToMetadataID: "column-a", Relationship: "contains"},
				{FromMetadataID: "column-b", ToMetadataID: "sibling-meta", Relationship: "upstream"},
				{FromMetadataID: "table-meta", ToMetadataID: "column-b", Relationship: "contains"},
			} {
				if !slices.Contains(capture.Edges, edge) {
					t.Errorf("confirmed edge %+v was lost: %+v", edge, capture.Edges)
				}
			}
		})
	}
}

func fieldPagingOwnerResponse(fields string, total int, more bool, cursor string) string {
	return fmt.Sprintf(`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-meta","luid":"ds-rest","name":"Sales","fieldsConnection":{"totalCount":%d,"pageInfo":{"hasNextPage":%t,"endCursor":%q},"nodes":%s}}]}}}`, total, more, cursor, fields)
}

func TestCaptureLineageInlineEvidenceHonorsGraphBoundsBeforeContinuation(t *testing.T) {
	for _, bound := range []string{"nodes", "edges"} {
		t.Run(bound, func(t *testing.T) {
			fields := make([]string, 100)
			for i := range fields {
				references := make([]string, 100)
				for j := range references {
					id := fmt.Sprintf("reference-%d", j)
					if bound == "nodes" {
						id = fmt.Sprintf("reference-%d-%d", i, j)
					}
					references[j] = fmt.Sprintf(`{"id":%q,"name":%q,"datasource":{"id":"ds-meta"}}`, id, id)
				}
				fields[i] = fmt.Sprintf(`{"__typename":"CalculatedField","id":"calc-%d","name":"Calc %d","datasource":{"id":"ds-meta"},"fieldsConnection":{"totalCount":101,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[%s]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}`, i, i, strings.Join(references, ","))
			}
			responses := append(fieldPagingAssets(t), fieldPagingOwnerResponse("["+strings.Join(fields, ",")+"]", len(fields), false, ""))
			server, requests := lineageTestServer(t, responses)
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			capture, err := client.CaptureLineage(t.Context(), CaptureRequest{Kind: KindPublishedDatasource, RESTLUID: "ds-rest", Direction: DirectionUpstream, Fields: true})
			if err != nil || capture.Complete || len(*requests) != len(responses) || len(capture.Nodes) > MaxLineageNodes || len(capture.Edges) > MaxLineageEdges {
				t.Fatalf("requests=%d, nodes=%d, edges=%d, complete=%t, err=%v", len(*requests), len(capture.Nodes), len(capture.Edges), capture.Complete, err)
			}
			warning := lineageNodeBoundWarning
			if bound == "edges" {
				warning = lineageEdgeBoundWarning
			}
			if !slices.Contains(capture.Warnings, warning) {
				t.Fatalf("missing bound warning %q: %v", warning, capture.Warnings)
			}
		})
	}
}

func fieldPagingAssets(t *testing.T) []string {
	t.Helper()
	return []string{
		`{"data":{"publishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"ds-meta","luid":"ds-rest","name":"Sales"}]}}}`,
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-meta", "ds-rest", "Sales", "upstreamDatasourcesConnection"),
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-meta", "ds-rest", "Sales", "upstreamFlowsConnection"),
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-meta", "ds-rest", "Sales", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "publishedDatasourcesConnection", "ds-meta", "ds-rest", "Sales", "upstreamTablesConnection"),
	}
}

func TestCaptureLineageFieldPagingRejectsNoProgressAndOversizedPages(t *testing.T) {
	empty := `{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}`
	field := func(id string) string {
		return fmt.Sprintf(`{"id":%q,"name":%q,"datasource":{"id":"ds-meta"},"upstreamColumnsConnection":%s}`, id, id, empty)
	}
	for _, tc := range []struct {
		name, page, want string
		pageSize         int
		retained         int
	}{
		{"owner_no_progress", fieldPagingOwnerResponse(`[]`, 1, true, "next"), "made no progress", 100, 1},
		{"owner_oversized", fieldPagingOwnerResponse("["+field("a")+","+field("b")+"]", 2, false, ""), "requested page size", 1, 1},
		{"owner_exceeds_total", fieldPagingOwnerResponse("["+field("a")+","+field("b")+"]", 1, false, ""), "more nodes than totalCount", 100, 2},
		{"references_no_progress", fieldPagingOwnerResponse(fmt.Sprintf(`[{"__typename":"CalculatedField","id":"calc","name":"Calc","datasource":{"id":"ds-meta"},"upstreamColumnsConnection":%s,"fieldsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[]}}]`, empty), 1, false, ""), "made no progress", 100, 2},
		{"columns_no_progress", fieldPagingOwnerResponse(`[{"id":"field","name":"Field","datasource":{"id":"ds-meta"},"upstreamColumnsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[]}}]`, 1, false, ""), "made no progress", 100, 2},
		{"columns_oversized", fieldPagingOwnerResponse(`[{"id":"field","name":"Field","datasource":{"id":"ds-meta"},"upstreamColumnsConnection":{"totalCount":101,"pageInfo":{"hasNextPage":false},"nodes":[`+strings.TrimSuffix(strings.Repeat(`{"id":"column","table":{"id":"outside"}},`, 101), ",")+`]}}]`, 1, false, ""), "requested page size", 100, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses := append(fieldPagingAssets(t), tc.page, `{"errors":[{"message":"unexpected later request","extensions":{"code":"ACCESS_DENIED"}}]}`)
			server, requests := lineageTestServer(t, responses)
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			capture, err := client.CaptureLineage(t.Context(), CaptureRequest{Kind: KindPublishedDatasource, RESTLUID: "ds-rest", Direction: DirectionUpstream, Fields: true, PageSize: tc.pageSize})
			if err == nil || !strings.Contains(err.Error(), tc.want) || capture.Complete || len(capture.Nodes) != tc.retained || len(*requests) != 6 {
				t.Fatalf("requests=%d, capture=%+v, err=%v", len(*requests), capture, err)
			}
		})
	}
}

func TestCaptureLineageFieldRequestsAreBoundedAcrossOutOfScopeRelationships(t *testing.T) {
	responses := fieldPagingAssets(t)
	fields := make([]string, 11)
	for i := range fields {
		fields[i] = fmt.Sprintf(`{"__typename":"CalculatedField","id":"calc-%d","name":"Calc %d","datasource":{"id":"ds-meta"},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"fieldsConnection":{"totalCount":100,"pageInfo":{"hasNextPage":true,"endCursor":"page-1"},"nodes":[{"id":"outside-%d-0","name":"Outside","datasource":{"id":"outside"}}]}}`, i, i, i)
	}
	responses = append(responses, fieldPagingOwnerResponse("["+strings.Join(fields, ",")+"]", len(fields), false, ""))
	for i := range fields {
		for page := 1; page < 100; page++ {
			field := fmt.Sprintf(`[{"id":"calc-%d","name":"Calc %d","datasource":{"id":"ds-meta"},"fieldsConnection":{"totalCount":100,"pageInfo":{"hasNextPage":%t,"endCursor":"page-%d"},"nodes":[{"id":"outside-%d-%d","name":"Outside","datasource":{"id":"outside"}}]}}]`, i, i, page < 99, page+1, i, page)
			responses = append(responses, fieldPagingOwnerResponse(field, 1, false, ""))
		}
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(t.Context(), CaptureRequest{Kind: KindPublishedDatasource, RESTLUID: "ds-rest", Direction: DirectionUpstream, Fields: true})
	if err == nil || !strings.Contains(err.Error(), "1000-request bound") || capture.Complete || len(*requests) != 1005 || len(capture.Nodes) != 12 || len(capture.Edges) != 11 {
		t.Fatalf("requests=%d, nodes=%d, edges=%d, complete=%t, err=%v", len(*requests), len(capture.Nodes), len(capture.Edges), capture.Complete, err)
	}
}

func TestCaptureLineageNestedFieldPageBoundRetainsConfirmedOwner(t *testing.T) {
	responses := fieldPagingAssets(t)
	field := `[{"__typename":"CalculatedField","id":"calc","name":"Calc","datasource":{"id":"ds-meta"},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"fieldsConnection":{"totalCount":101,"pageInfo":{"hasNextPage":true,"endCursor":"page-1"},"nodes":[{"id":"outside-0","name":"Outside","datasource":{"id":"outside"}}]}}]`
	responses = append(responses, fieldPagingOwnerResponse(field, 1, false, ""))
	for page := 1; page < 100; page++ {
		field = fmt.Sprintf(`[{"id":"calc","name":"Calc","datasource":{"id":"ds-meta"},"fieldsConnection":{"totalCount":101,"pageInfo":{"hasNextPage":true,"endCursor":"page-%d"},"nodes":[{"id":"outside-%d","name":"Outside","datasource":{"id":"outside"}}]}}]`, page+1, page)
		responses = append(responses, fieldPagingOwnerResponse(field, 1, false, ""))
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(t.Context(), CaptureRequest{Kind: KindPublishedDatasource, RESTLUID: "ds-rest", Direction: DirectionUpstream, Fields: true})
	if err == nil || !strings.Contains(err.Error(), "100-page bound") || capture.Complete || len(*requests) != 105 || len(capture.Nodes) != 2 || len(capture.Edges) != 1 || FailureFor(CaptureRequest{}, err).Relation != "fieldsConnection" {
		t.Fatalf("requests=%d, capture=%+v, err=%v", len(*requests), capture, err)
	}
}

func TestCaptureLineageContinuedColumnsRespectRequestedPageSize(t *testing.T) {
	responses := fieldPagingAssets(t)
	field := `[{"id":"field","name":"Field","datasource":{"id":"ds-meta"},"upstreamColumnsConnection":{"totalCount":3,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"column-0","table":{"id":"outside"}}]}}]`
	responses = append(responses, fieldPagingOwnerResponse(field, 1, false, ""))
	field = `[{"id":"field","name":"Field","datasource":{"id":"ds-meta"},"upstreamColumnsConnection":{"totalCount":3,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"column-1","table":{"id":"outside"}},{"id":"column-2","table":{"id":"outside"}}]}}]`
	responses = append(responses, fieldPagingOwnerResponse(field, 1, false, ""))
	server, requests := lineageTestServer(t, responses)
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(t.Context(), CaptureRequest{Kind: KindPublishedDatasource, RESTLUID: "ds-rest", Direction: DirectionUpstream, Fields: true, PageSize: 1})
	if err == nil || !strings.Contains(err.Error(), "requested page size 1") || capture.Complete || len(*requests) != 7 || len(capture.Nodes) != 2 || len(capture.Edges) != 1 || FailureFor(CaptureRequest{}, err).Relation != "upstreamColumnsConnection" {
		t.Fatalf("requests=%d, capture=%+v, err=%v", len(*requests), capture, err)
	}
}

func TestCaptureLineageEmbeddedFieldContinuationUsesSingletonIDLists(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-rest","name":"Book"}]}}}`,
		emptyLineageRelationResponse(t, "workbooksConnection", "wb-meta", "wb-rest", "Book", "upstreamDatasourcesConnection"),
		emptyLineageRelationResponse(t, "workbooksConnection", "wb-meta", "wb-rest", "Book", "upstreamDatabasesConnection"),
		emptyLineageRelationResponse(t, "workbooksConnection", "wb-meta", "wb-rest", "Book", "upstreamTablesConnection"),
		`{"data":{"workbooksConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-rest","name":"Book","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local"}]}}]}}}`,
		`{"data":{"embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","fieldsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"CalculatedField","id":"calc","name":"Calc","datasource":{"id":"embedded-meta"},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"fieldsConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"reference-1","name":"Reference 1","datasource":{"id":"embedded-meta"}}]}}]}}]}}}`,
		`{"data":{"embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","fieldsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"calc","name":"Calc","datasource":{"id":"embedded-meta"},"fieldsConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"reference-2","name":"Reference 2","datasource":{"id":"embedded-meta"}}]}}]}}]}}}`,
	}
	server, requests := lineageTestServer(t, responses)
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	capture, err := client.CaptureLineage(t.Context(), CaptureRequest{Kind: KindWorkbook, RESTLUID: "wb-rest", Direction: DirectionUpstream, Fields: true})
	if err != nil || !capture.Complete || len(capture.Nodes) != 5 || len(capture.Edges) != 6 || len(*requests) != 7 {
		t.Fatalf("requests=%d, capture=%+v, err=%v", len(*requests), capture, err)
	}
	if (*requests)[5].Variables["rootMetadataID"] != "embedded-meta" || (*requests)[6].Variables["rootMetadataID"] != "embedded-meta" || (*requests)[6].Variables["fieldID"] != "calc" || (*requests)[6].Variables["after"] != "next" {
		t.Fatalf("owner and field selectors changed: %+v", *requests)
	}
}
