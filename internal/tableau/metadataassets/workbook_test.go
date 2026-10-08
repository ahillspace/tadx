package metadataassets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
)

func TestWorkbookDatasourcesUsesExactWorkbookAndEmbeddedMetadataIDs(t *testing.T) {
	seen := map[string]bool{}
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(q.Query, "workbooksConnection") {
			if q.Variables["luid"] != "wb-1" {
				t.Fatalf("workbook selector: %#v", q.Variables)
			}
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","description":"Workbook meaning","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","hasExtracts":false}]}}]}}}`)
			return
		}
		if q.Variables["id"] != "embedded-meta" {
			t.Fatalf("embedded selector: %#v", q.Variables)
		}
		for name := range map[string]bool{"fieldsConnection": true, "parentPublishedDatasourcesConnection": true, "upstreamTablesConnection": true, "upstreamDatabasesConnection": true, "upstreamFlowsConnection": true, "downstreamSheetsConnection": true} {
			if strings.Contains(q.Query, "items:"+name) {
				seen[name] = true
				fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`)
				return
			}
		}
		t.Fatalf("unexpected query: %s", q.Query)
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err != nil || !got.Complete || got.Identity.MetadataID != "wb-meta" || len(got.Datasources) != 1 || got.Datasources[0].Identity.MetadataID != "embedded-meta" || got.Datasources[0].Kind != "embedded" || got.Description == nil || *got.Description != "Workbook meaning" || len(seen) != 6 {
		t.Fatalf("got %+v, seen %v, error %v", got, seen, err)
	}
}

func TestWorkbookLocalFieldDescriptionAndNestedColumnPagination(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.Contains(q.Query, "workbooksConnection"):
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local"}]}}]}}}`)
		case strings.Contains(q.Query, "query FieldColumns"):
			calls++
			if !strings.Contains(q.Query, "$parent:ID!") || q.Variables["parent"] != "embedded-meta" || q.Variables["field"] != "field-meta" || q.Variables["after"] != "next" {
				t.Fatalf("column page request: %+v", q)
			}
			fmt.Fprint(w, `{"data":{"embedded":{"nodes":[{"id":"embedded-meta","items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"field-meta","upstreamColumnsConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"col-2","name":"Column 2"}]}}]}}]}}}`)
		case strings.Contains(q.Query, "items:fieldsConnection"):
			fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"field-meta","name":"Local Name","fullyQualifiedName":"[Local Name]","__typename":"DatasourceField","description":"Local description","folderName":"Local folder","isHidden":false,"remoteField":{"id":"remote-meta","name":"Published Name","__typename":"ColumnField"},"descriptionInherited":[],"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"upstreamColumnsConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"col-1","name":"Column 1"}]}}]}}]}}}`)
		default:
			fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`)
		}
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err != nil || !got.Complete || calls != 1 || len(got.Datasources) != 1 || len(got.Datasources[0].Fields) != 1 {
		t.Fatalf("got %+v, calls %d, error %v", got, calls, err)
	}
	f := got.Datasources[0].Fields[0]
	if f.Description == nil || *f.Description != "Local description" || f.RemoteField == nil || f.RemoteField.MetadataID != "remote-meta" || len(f.UpstreamColumns) != 2 || f.Hidden == nil || *f.Hidden {
		t.Fatalf("field %+v", f)
	}
}

func TestWorkbookPartialFieldPagePreservesObservedField(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(q.Query, "workbooksConnection") {
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local"}]}}]}}}`)
			return
		}
		if q.Variables["after"] == "next" {
			fmt.Fprint(w, `{"errors":[{"message":"access denied"}]}`)
			return
		}
		fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"field-meta","name":"Sales","fullyQualifiedName":"[Sales]","description":"Local","descriptionInherited":[],"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}]}}}`)
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || len(got.Datasources) != 1 || got.Datasources[0].Complete || len(got.Datasources[0].Fields) != 1 {
		t.Fatalf("got %+v, error %v", got, err)
	}
}

func TestWorkbookRepeatedPublishedParentIsFetchedOnce(t *testing.T) {
	publishedCalls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.Contains(q.Query, "workbooksConnection"):
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"Local 1"},{"id":"emb-2","name":"Local 2"}]}}]}}}`)
		case strings.Contains(q.Query, "publishedDatasourcesConnection"):
			publishedCalls++
			if q.Variables["parent"] != "published-1" {
				t.Fatalf("published selector: %+v", q)
			}
			fmt.Fprint(w, `{"data":{"parents":{"nodes":[{"id":"published-meta","luid":"published-1","name":"Published","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`)
		case strings.Contains(q.Query, "items:parentPublishedDatasourcesConnection"):
			fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":%q,"name":"Local","items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"published-meta","luid":"published-1","name":"Published"}]}}]}}}`, q.Variables["id"])
		default:
			fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":%q,"name":"Local","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`, q.Variables["id"])
		}
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err != nil || !got.Complete || len(got.Datasources) != 3 || got.Datasources[2].Kind != "published" || publishedCalls != 3 {
		t.Fatalf("got %+v, published calls %d, error %v", got, publishedCalls, err)
	}
}

func TestWorkbookRejectsMismatchedMetadataIdentity(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"other","name":"Book","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`)
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || len(got.Datasources) != 0 {
		t.Fatalf("got %+v, error %v", got, err)
	}
}

func TestWorkbookParentConnectionPagesIndependently(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		if q.Variables["id"] != "embedded-meta" || !strings.Contains(q.Query, "items:parentPublishedDatasourcesConnection") {
			t.Fatalf("query %+v", q)
		}
		if q.Variables["after"] == nil {
			fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"parent-1","luid":"ds-1","name":"First"}]}}]}}}`)
			return
		}
		if q.Variables["after"] != "next" {
			t.Fatalf("cursor %+v", q.Variables)
		}
		fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"parent-2","luid":"ds-2","name":"Second"}]}}]}}}`)
	})
	requests := 0
	requestID := ""
	var ids []string
	rows, err := c.workbookRelation(t.Context(), "embedded-meta", "parentPublishedDatasourcesConnection", "id luid name", &requests, &requestID, func(n graphNode, _ tableau.Response) error { ids = append(ids, n.ID); return nil })
	if err != nil || requests != 2 || len(rows) != 2 || len(ids) != 2 || ids[0] != "parent-1" || ids[1] != "parent-2" {
		t.Fatalf("rows %+v, ids %v, requests %d, error %v", rows, ids, requests, err)
	}
}

func TestWorkbookPublishedReadsShareAggregateRequestBudget(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatalf("request exceeded workbook budget: %s", r.URL) })
	budget := 400
	_, err := c.datasourceFieldDescriptions(t.Context(), "ds-1", &budget, true)
	if err == nil || budget != 400 {
		t.Fatalf("budget %d, error %v", budget, err)
	}
	_, err = c.datasourceUpstream(t.Context(), "ds-1", &budget)
	if err == nil || budget != 400 {
		t.Fatalf("budget %d, error %v", budget, err)
	}
}

func TestWorkbookRejectsConflictingRepeatedParentIdentity(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(q.Query, "workbooksConnection") {
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First"},{"id":"emb-2","name":"Second"}]}}]}}}`)
			return
		}
		if strings.Contains(q.Query, "publishedDatasourcesConnection") {
			t.Fatal("published enrichment started before repeated identity validation")
		}
		items := `{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}`
		if strings.Contains(q.Query, "items:parentPublishedDatasourcesConnection") {
			name := "Published"
			if q.Variables["id"] == "emb-2" {
				name = "Changed"
			}
			items = fmt.Sprintf(`{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"published-meta","luid":"ds-1","name":%q}]}`, name)
		}
		fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":%q,"name":"Local","items":%s}]}}}`, q.Variables["id"], items)
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || len(got.Datasources) != 2 {
		t.Fatalf("got %+v, error %v", got, err)
	}
}

func TestWorkbookFieldRelationsPageIndependently(t *testing.T) {
	for _, relation := range []string{"relatedSheets", "referencedFields"} {
		t.Run(relation, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				var q struct {
					Query     string         `json:"query"`
					Variables map[string]any `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Fatal(err)
				}
				if q.Variables["parent"] != "embedded-meta" || q.Variables["field"] != "field-meta" || q.Variables["after"] != "next" {
					t.Fatalf("query variables %+v", q.Variables)
				}
				if relation == "relatedSheets" && !strings.Contains(q.Query, "items:downstreamSheetsConnection") {
					t.Fatalf("query %s", q.Query)
				}
				if relation == "referencedFields" && !strings.Contains(q.Query, "items:fieldsConnection") {
					t.Fatalf("query %s", q.Query)
				}
				fmt.Fprint(w, `{"data":{"embedded":{"nodes":[{"id":"embedded-meta","items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"field-meta","items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"second","name":"Second","luid":"sheet-2"}]}}]}}]}}}`)
			})
			initial := &graphPage{Nodes: []graphNode{{ID: "first", Name: "First", LUID: "sheet-1"}}, Total: new(2), Info: &graphInfo{More: new(true), Cursor: new("next")}}
			budget := 0
			rows, err := c.fieldRelation(t.Context(), "embedded-meta", "field-meta", relation, initial, &budget, true, nil)
			if err != nil || budget != 1 || len(rows) != 2 || rows[0].ID != "first" || rows[1].ID != "second" {
				t.Fatalf("rows %+v, requests %d, error %v", rows, budget, err)
			}
		})
	}
}

func TestWorkbookContinuesAfterOnePublishedParentFails(t *testing.T) {
	secondRead := false
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.Contains(q.Query, "workbooksConnection"):
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local"}]}}]}}}`)
		case strings.Contains(q.Query, "publishedDatasourcesConnection"):
			parent := q.Variables["parent"].(string)
			if parent == "ds-2" {
				secondRead = true
			}
			if parent == "ds-1" && strings.Contains(q.Query, "items:fieldsConnection") {
				fmt.Fprint(w, `{"errors":[{"message":"parent unavailable"}]}`)
				return
			}
			fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"id":%q,"luid":%q,"name":%q,"tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`, "meta-"+parent, parent, parent)
		case strings.Contains(q.Query, "items:parentPublishedDatasourcesConnection"):
			fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta-ds-1","luid":"ds-1","name":"ds-1"},{"id":"meta-ds-2","luid":"ds-2","name":"ds-2"}]}}]}}}`)
		default:
			fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`)
		}
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || !secondRead || len(got.Datasources) != 3 || got.Datasources[1].Complete || !got.Datasources[2].Complete {
		t.Fatalf("got %+v, secondRead %v, error %v", got, secondRead, err)
	}
}
