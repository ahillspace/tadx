package metadataassets

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

func workbookGraphRequest(t *testing.T, r *http.Request) (*ast.Field, map[string]any) {
	t.Helper()
	var request struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	doc, err := parser.ParseQuery(&ast.Source{Input: request.Query})
	if err != nil {
		t.Fatal(err)
	}
	return doc.Operations[0].SelectionSet[0].(*ast.Field), request.Variables
}

func workbookItemsField(t *testing.T, root *ast.Field) *ast.Field {
	t.Helper()
	for _, selection := range root.SelectionSet {
		field, ok := selection.(*ast.Field)
		if !ok || field.Name != "nodes" {
			continue
		}
		for _, selection := range field.SelectionSet {
			field, ok := selection.(*ast.Field)
			if ok && field.Alias == "items" {
				return field
			}
		}
	}
	t.Fatal("query omits datasource items selection")
	return nil
}

func TestWorkbookDetailFailurePreservesAllObservedEmbeddedIdentities(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		root, variables := workbookGraphRequest(t, r)
		calls++
		if root.Name == "workbooksConnection" {
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":3,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First","hasExtracts":true},{"id":"emb-2","name":"Second","hasExtracts":false},{"id":"emb-3","name":"Third","hasUserReference":true}]}}]}}}`)
			return
		}
		if variables["id"] != "emb-1" {
			t.Errorf("unexpected detail selector: %v", variables)
		}
		fmt.Fprint(w, `{"errors":[{"message":"embedded fields unavailable"}]}`)
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || got.Identity.MetadataID != "wb-meta" || len(got.Datasources) != 3 || calls != 2 {
		t.Fatalf("got %+v, calls %d, error %v", got, calls, err)
	}
	for i, datasource := range got.Datasources {
		if datasource.Identity.MetadataID != fmt.Sprintf("emb-%d", i+1) || datasource.Complete {
			t.Fatalf("confirmed datasource %d: %+v", i, datasource)
		}
	}
	if got.Datasources[1].HasExtracts == nil || *got.Datasources[1].HasExtracts || got.Datasources[2].HasUserReference == nil || !*got.Datasources[2].HasUserReference {
		t.Fatalf("confirmed embedded metadata omitted: %+v", got.Datasources)
	}
}

func TestWorkbookInventoryPaginationPreservesEmbeddedMetadata(t *testing.T) {
	inventoryCalls := 0
	detailCalls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		root, variables := workbookGraphRequest(t, r)
		if root.Name == "workbooksConnection" {
			inventoryCalls++
			filter := root.Arguments.ForName("filter").Value.Children.ForName("luid")
			items := workbookItemsField(t, root)
			if filter.Kind != ast.Variable || filter.Raw != "luid" || variables["luid"] != "wb-1" || root.Arguments.ForName("first").Value.Raw != "2" || items.Arguments.ForName("first").Value.Raw != "100" || items.Arguments.ForName("after").Value.Raw != "after" {
				t.Errorf("inventory selectors or bounds changed: %+v, %v", root.Arguments, variables)
			}
			if variables["after"] == nil {
				fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","description":"Purpose","tagsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"name":"Workbook tag"}]},"items":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"next-source"},"nodes":[{"id":"emb-1","name":"First","hasExtracts":true,"hasUserReference":false,"containsUnsupportedCustomSql":true,"extractLastUpdateTime":"2026-01-01T01:00:00Z","extractLastRefreshTime":"2026-01-01T02:00:00Z","extractLastIncrementalUpdateTime":"2026-01-01T03:00:00Z"}]}}]}}}`)
				return
			}
			if variables["after"] != "next-source" {
				t.Errorf("inventory cursor changed: %v", variables)
			}
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","description":"Purpose","items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-2","name":"Second","hasExtracts":false}]}}]}}}`)
			return
		}
		detailCalls++
		filter := root.Arguments.ForName("filter").Value.Children.ForName("idWithin")
		items := workbookItemsField(t, root)
		if root.Name != "embeddedDatasourcesConnection" || filter == nil || filter.Kind != ast.ListValue || len(filter.Children) != 1 || filter.Children[0].Value.Kind != ast.Variable || filter.Children[0].Value.Raw != "id" || (variables["id"] != "emb-1" && variables["id"] != "emb-2") || variables["after"] != nil || items.Arguments.ForName("first").Value.Raw != "100" {
			t.Errorf("detail selectors or bounds changed: %+v, %v", root.Arguments, variables)
		}
		fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":%q,"name":"Local","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`, variables["id"])
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err != nil || !got.Complete || inventoryCalls != 2 || detailCalls != 12 || len(got.Datasources) != 2 || len(got.Tags) != 1 || got.Tags[0] != "Workbook tag" || !got.TagsObserved {
		t.Fatalf("got %+v, inventory %d, details %d, error %v", got, inventoryCalls, detailCalls, err)
	}
	first := got.Datasources[0]
	if !first.Complete || first.HasExtracts == nil || !*first.HasExtracts || first.HasUserReference == nil || *first.HasUserReference || first.ContainsUnsupportedCustomSQL == nil || !*first.ContainsUnsupportedCustomSQL || first.ExtractLastUpdateTime != "2026-01-01T01:00:00Z" || first.ExtractLastRefreshTime != "2026-01-01T02:00:00Z" || first.ExtractLastIncrementalUpdateTime != "2026-01-01T03:00:00Z" {
		t.Fatalf("embedded metadata omitted: %+v", first)
	}
}

func TestWorkbookFieldFailurePreservesObservedPublishedGroups(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		root, variables := workbookGraphRequest(t, r)
		calls++
		if root.Name == "workbooksConnection" {
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First"}]}}]}}}`)
			return
		}
		if root.Name != "embeddedDatasourcesConnection" || variables["id"] != "emb-1" {
			t.Errorf("unexpected detail request: %s, %v", root.Name, variables)
		}
		if workbookItemsField(t, root).Name == "parentPublishedDatasourcesConnection" {
			fmt.Fprint(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First","items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"parent-1","luid":"ds-1","name":"Published 1"},{"id":"parent-2","luid":"ds-2","name":"Published 2"}]}}]}}}`)
			return
		}
		fmt.Fprint(w, `{"errors":[{"message":"embedded fields unavailable"}]}`)
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || len(got.Datasources) != 3 || len(got.Datasources[0].Parents) != 2 || calls != 3 {
		t.Fatalf("got %+v, calls %d, error %v", got, calls, err)
	}
	for i, datasource := range got.Datasources[1:] {
		if datasource.Kind != "published" || datasource.Identity.MetadataID != fmt.Sprintf("parent-%d", i+1) || datasource.Identity.LUID != fmt.Sprintf("ds-%d", i+1) || datasource.Complete || len(datasource.Fields) != 0 {
			t.Fatalf("confirmed published datasource %d: %+v", i, datasource)
		}
	}
}

func TestWorkbookInventoryBeyondFirstHundredSurvivesDetailFailure(t *testing.T) {
	inventoryCalls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		root, variables := workbookGraphRequest(t, r)
		if root.Name != "workbooksConnection" {
			fmt.Fprint(w, `{"errors":[{"message":"detail unavailable"}]}`)
			return
		}
		inventoryCalls++
		nodes := []graphNode{}
		if variables["after"] == nil {
			for i := range 100 {
				nodes = append(nodes, graphNode{ID: fmt.Sprintf("emb-%03d", i), Name: fmt.Sprintf("Local %d", i)})
			}
		} else {
			if variables["after"] != "next-hundred" {
				t.Errorf("inventory cursor: %v", variables)
			}
			nodes = append(nodes, graphNode{ID: "emb-100", Name: "Local 100"})
		}
		items := &graphPage{Nodes: nodes, Total: new(101), Info: &graphInfo{More: new(variables["after"] == nil), Cursor: new("next-hundred")}}
		workbooks := graphPage{Nodes: []graphNode{{ID: "wb-meta", LUID: "wb-1", Name: "Book", Items: items}}, Total: new(1), Info: &graphInfo{More: new(false)}}
		if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"workbooks": workbooks}}); err != nil {
			t.Error(err)
		}
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || inventoryCalls != 2 || len(got.Datasources) != 101 || got.Datasources[100].Identity.MetadataID != "emb-100" {
		t.Fatalf("observed %d sources, inventory %d, complete %v, error %v", len(got.Datasources), inventoryCalls, got.Complete, err)
	}
	for _, datasource := range got.Datasources {
		if datasource.Complete {
			t.Fatalf("detail coverage invented: %+v", datasource)
		}
	}
}

func TestWorkbookRequestWidePublishedFailurePreservesUnqueriedGroups(t *testing.T) {
	publishedCalls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		root, variables := workbookGraphRequest(t, r)
		switch root.Name {
		case "workbooksConnection":
			fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First"}]}}]}}}`)
		case "publishedDatasourcesConnection":
			publishedCalls++
			if variables["parent"] != "ds-1" {
				t.Errorf("request-wide failure did not stop published enrichment: %v", variables)
			}
			http.Error(w, "access denied", http.StatusForbidden)
		case "embeddedDatasourcesConnection":
			items := `{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}`
			if workbookItemsField(t, root).Name == "parentPublishedDatasourcesConnection" {
				items = `{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"parent-1","luid":"ds-1","name":"Published 1"},{"id":"parent-2","luid":"ds-2","name":"Published 2"}]}`
			}
			fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First","items":%s}]}}}`, items)
		default:
			t.Errorf("unexpected root connection: %s", root.Name)
		}
	})
	got, err := c.WorkbookDatasources(t.Context(), "wb-1")
	if err == nil || got.Complete || len(got.Datasources) != 3 || !got.Datasources[0].Complete || got.Datasources[1].Complete || got.Datasources[2].Complete || got.Datasources[2].Identity.MetadataID != "parent-2" || publishedCalls != 1 {
		t.Fatalf("got %+v, published calls %d, error %v", got, publishedCalls, err)
	}
}

func TestWorkbookRejectsPublishedEnrichmentUnderChangedIdentity(t *testing.T) {
	for _, changed := range []string{"metadata-id", "name", "luid"} {
		t.Run(changed, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				root, _ := workbookGraphRequest(t, r)
				switch root.Name {
				case "workbooksConnection":
					fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First"}]}}]}}}`)
				case "embeddedDatasourcesConnection":
					items := `{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}`
					if workbookItemsField(t, root).Name == "parentPublishedDatasourcesConnection" {
						items = `{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"parent-1","luid":"ds-1","name":"Published"}]}`
					}
					fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First","items":%s}]}}}`, items)
				case "publishedDatasourcesConnection":
					id, name, luid := "parent-1", "Published", "ds-1"
					switch changed {
					case "metadata-id":
						id = "foreign-parent"
					case "name":
						name = "Foreign name"
					case "luid":
						luid = "foreign-luid"
					}
					fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"id":%q,"luid":%q,"name":%q,"description":"Foreign purpose","tagsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"name":"Foreign tag"}]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"foreign-field","name":"Foreign field","fullyQualifiedName":"[Foreign field]","description":"Foreign field purpose","descriptionInherited":[],"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}]}}}`, id, luid, name)
				default:
					t.Errorf("unexpected root: %s", root.Name)
				}
			})
			got, err := c.WorkbookDatasources(t.Context(), "wb-1")
			if err == nil || got.Complete || len(got.Datasources) != 2 {
				t.Fatalf("got %+v, error %v", got, err)
			}
			published := got.Datasources[1]
			if published.Identity.MetadataID != "parent-1" || published.Identity.LUID != "ds-1" || published.Identity.Name != "Published" || published.Complete || len(published.Fields) != 0 || published.Description != nil || len(published.Tags) != 0 || published.TagsObserved {
				t.Fatalf("mismatched enrichment attached to confirmed identity: %+v", published)
			}
		})
	}
}

func TestWorkbookEmbeddedAndPublishedAggregateColumnBounds(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		for _, fieldCount := range []int{100, 101} {
			t.Run(fmt.Sprintf("embedded=%t/fields=%d", embedded, fieldCount), func(t *testing.T) {
				columns := make([]map[string]any, 100)
				for i := range columns {
					columns[i] = map[string]any{"id": fmt.Sprintf("column-%03d", i), "name": fmt.Sprintf("Column %d", i)}
				}
				c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
					root, variables := workbookGraphRequest(t, r)
					if root.Name == "workbooksConnection" {
						fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First"}]}}]}}}`)
						return
					}
					nodes := []map[string]any{}
					total := 0
					more := false
					if workbookItemsField(t, root).Name == "fieldsConnection" {
						total = fieldCount
						start, count := 0, min(fieldCount, 100)
						if variables["after"] != nil {
							if variables["after"] != "next-field" {
								t.Errorf("field cursor: %v", variables)
							}
							start, count = 100, fieldCount-100
						}
						more = start+count < fieldCount
						for i := range count {
							nodes = append(nodes, map[string]any{
								"id": fmt.Sprintf("field-%03d", start+i), "name": fmt.Sprintf("Field %d", start+i), "fullyQualifiedName": fmt.Sprintf("[Field %d]", start+i),
								"descriptionInherited":      []any{},
								"relatedSheets":             map[string]any{"totalCount": 0, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}},
								"upstreamColumnsConnection": map[string]any{"totalCount": 100, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": columns},
							})
						}
					}
					parent := map[string]any{"id": "parent-1", "luid": "ds-1", "name": "Published", "items": map[string]any{"totalCount": total, "pageInfo": map[string]any{"hasNextPage": more, "endCursor": "next-field"}, "nodes": nodes}}
					container := "parents"
					if embedded {
						container, parent["id"], parent["name"] = "embedded", "emb-1", "First"
					}
					if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{container: map[string]any{"totalCount": 1, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{parent}}}}); err != nil {
						t.Error(err)
					}
				})
				var fields []value.FieldDescription
				var complete bool
				var err error
				if embedded {
					got, readErr := c.WorkbookDatasources(t.Context(), "wb-1")
					err, complete = readErr, got.Complete
					if len(got.Datasources) != 1 {
						t.Fatalf("embedded identities: %+v", got)
					}
					fields = got.Datasources[0].Fields
					if got.Datasources[0].Complete != complete {
						t.Fatalf("embedded coverage differs from workbook: %+v", got)
					}
				} else {
					got, readErr := c.DatasourceFieldDescriptions(t.Context(), "ds-1")
					err, complete, fields = readErr, got.Complete, got.Fields
				}
				observedColumns := 0
				for _, field := range fields {
					observedColumns += len(field.UpstreamColumns)
				}
				if fieldCount == 100 {
					if err != nil || !complete || len(fields) != 100 || observedColumns != 10000 {
						t.Fatalf("boundary fields %d, columns %d, complete %v, error %v", len(fields), observedColumns, complete, err)
					}
					return
				}
				if _, ok := errors.AsType[*ConstraintError](err); !ok || complete || observedColumns != 10000 || len(fields) != 100 {
					t.Fatalf("overflow fields %d, columns %d, complete %v, error %v", len(fields), observedColumns, complete, err)
				}
			})
		}
	}
}

func TestPublishedFieldPaginationRejectsChangedParentIdentity(t *testing.T) {
	for _, workbook := range []bool{false, true} {
		for _, changed := range []string{"metadata-id", "name"} {
			t.Run(fmt.Sprintf("workbook=%t/%s", workbook, changed), func(t *testing.T) {
				fieldCalls := 0
				c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
					root, variables := workbookGraphRequest(t, r)
					switch root.Name {
					case "workbooksConnection":
						fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First"}]}}]}}}`)
					case "embeddedDatasourcesConnection":
						items := `{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}`
						if workbookItemsField(t, root).Name == "parentPublishedDatasourcesConnection" {
							items = `{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"parent-1","luid":"ds-1","name":"Published"}]}`
						}
						fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"First","items":%s}]}}}`, items)
					case "publishedDatasourcesConnection":
						if workbookItemsField(t, root).Name != "fieldsConnection" {
							fmt.Fprint(w, `{"data":{"parents":{"nodes":[{"id":"parent-1","luid":"ds-1","name":"Published","items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`)
							return
						}
						fieldCalls++
						id, name, fieldID := "parent-1", "Published", "first-field"
						more := variables["after"] == nil
						if !more {
							if variables["after"] != "next-field" || variables["parent"] != "ds-1" {
								t.Errorf("published pagination selectors: %v", variables)
							}
							fieldID = "foreign-field"
							if changed == "metadata-id" {
								id = "foreign-parent"
							} else {
								name = "Foreign name"
							}
						}
						fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"id":%q,"luid":"ds-1","name":%q,"description":"Original purpose","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":2,"pageInfo":{"hasNextPage":%t,"endCursor":"next-field"},"nodes":[{"id":%q,"name":%q,"fullyQualifiedName":"[Field]","descriptionInherited":[],"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}]}}}`, id, name, more, fieldID, fieldID)
					default:
						t.Errorf("unexpected root: %s", root.Name)
					}
				})
				var fields []value.FieldDescription
				var complete bool
				var err error
				if workbook {
					got, readErr := c.WorkbookDatasources(t.Context(), "wb-1")
					err, complete = readErr, got.Complete
					if len(got.Datasources) != 2 || got.Datasources[1].Identity.MetadataID != "parent-1" || got.Datasources[1].Identity.Name != "Published" || got.Datasources[1].Complete {
						t.Fatalf("published group: %+v, error %v", got, err)
					}
					fields = got.Datasources[1].Fields
				} else {
					got, readErr := c.DatasourceFieldDescriptions(t.Context(), "ds-1")
					err, complete, fields = readErr, got.Complete, got.Fields
					if got.Identity.MetadataID != "parent-1" || got.Identity.Name != "Published" || got.Description == nil || *got.Description != "Original purpose" {
						t.Fatalf("published identity: %+v, error %v", got, err)
					}
				}
				if err == nil || complete || fieldCalls != 2 || len(fields) != 1 || fields[0].MetadataID != "first-field" {
					t.Fatalf("fields %+v, calls %d, complete %v, error %v", fields, fieldCalls, complete, err)
				}
			})
		}
	}
}

func TestFieldRelationSingletonFilterKeepsExactIdentityGuards(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		for _, relation := range []string{"relatedSheets", "referencedFields"} {
			for _, wrongField := range []bool{false, true} {
				t.Run(fmt.Sprintf("embedded=%t/%s/wrong-field=%t", embedded, relation, wrongField), func(t *testing.T) {
					parentID := "ds-1"
					if embedded {
						parentID = "embedded-meta"
					}
					c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
						root, variables := workbookGraphRequest(t, r)
						filter := workbookItemsField(t, root).Arguments.ForName("filter").Value.Children.ForName("idWithin")
						if filter == nil || filter.Kind != ast.ListValue || len(filter.Children) != 1 || filter.Children[0].Value.Kind != ast.Variable || filter.Children[0].Value.Raw != "field" || variables["field"] != "field-meta" || variables["parent"] != parentID || variables["after"] != "next" {
							t.Errorf("field filter or variables: %+v, %v", filter, variables)
						}
						parentFilter := root.Arguments.ForName("filter").Value.Children
						if embedded {
							selected := parentFilter.ForName("idWithin")
							if selected == nil || selected.Kind != ast.ListValue || len(selected.Children) != 1 || selected.Children[0].Value.Kind != ast.Variable || selected.Children[0].Value.Raw != "parent" {
								t.Errorf("embedded parent filter: %+v", parentFilter)
							}
						} else if selected := parentFilter.ForName("luid"); selected == nil || selected.Kind != ast.Variable || selected.Raw != "parent" {
							t.Errorf("published parent LUID filter: %+v", parentFilter)
						}
						fieldID := "field-meta"
						if wrongField {
							fieldID = "foreign-field"
						}
						container, selector := "parents", "luid"
						if embedded {
							container, selector = "embedded", "id"
						}
						fmt.Fprintf(w, `{"data":{%q:{"nodes":[{%q:%q,"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":%q,"items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"second","name":"Second","luid":"sheet-2"}]}}]}}]}}}`, container, selector, parentID, fieldID)
					})
					initial := &graphPage{Nodes: []graphNode{{ID: "first", Name: "First", LUID: "sheet-1"}}, Total: new(2), Info: &graphInfo{More: new(true), Cursor: new("next")}}
					requests := 0
					rows, err := c.fieldRelation(t.Context(), parentID, "field-meta", relation, initial, &requests, embedded, nil)
					if wrongField {
						if err == nil || requests != 1 || len(rows) != 1 || rows[0].ID != "first" {
							t.Fatalf("wrong field accepted: rows %+v, requests %d, error %v", rows, requests, err)
						}
						return
					}
					if err != nil || requests != 1 || len(rows) != 2 || rows[0].ID != "first" || rows[1].ID != "second" {
						t.Fatalf("rows %+v, requests %d, error %v", rows, requests, err)
					}
				})
			}
		}
	}
}

func TestFieldContinuationFailurePreservesCurrentPageEvidence(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		for _, failure := range []string{"columns", "relatedSheets", "referencedFields"} {
			t.Run(fmt.Sprintf("embedded=%t/%s", embedded, failure), func(t *testing.T) {
				continuations := 0
				c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
					root, variables := workbookGraphRequest(t, r)
					if variables["field"] != nil {
						continuations++
						if variables["field"] != "field-a" || variables["after"] != "next" {
							t.Errorf("unexpected continuation: %v", variables)
						}
						fmt.Fprint(w, `{"errors":[{"message":"nested continuation unavailable"}]}`)
						return
					}
					if root.Name == "workbooksConnection" {
						fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Book","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"emb-1","name":"Local"}]}}]}}}`)
						return
					}
					page := func(nodes []map[string]any, more bool) map[string]any {
						return map[string]any{"totalCount": len(nodes) + map[bool]int{true: 1}[more], "pageInfo": map[string]any{"hasNextPage": more, "endCursor": "next"}, "nodes": nodes}
					}
					fields := []map[string]any{}
					if workbookItemsField(t, root).Name == "fieldsConnection" {
						for _, suffix := range []string{"a", "b"} {
							fields = append(fields, map[string]any{
								"__typename": "CalculatedField", "id": "field-" + suffix, "name": "Field " + suffix, "fullyQualifiedName": "[Field " + suffix + "]", "description": "Meaning " + suffix, "descriptionInherited": []any{},
								"upstreamColumnsConnection": page([]map[string]any{{"id": "column-" + suffix, "name": "Column " + suffix}}, suffix == "a" && failure == "columns"),
								"relatedSheets":             page([]map[string]any{{"id": "sheet-" + suffix, "name": "Sheet " + suffix}}, suffix == "a" && failure == "relatedSheets"),
								"referencedFields":          page([]map[string]any{{"id": "reference-" + suffix, "name": "Reference " + suffix}}, suffix == "a" && failure == "referencedFields"),
							})
						}
					}
					parent := map[string]any{"id": "published-meta", "luid": "ds-1", "name": "Published", "items": page(fields, false)}
					container := "parents"
					if embedded {
						container, parent["id"], parent["name"] = "embedded", "emb-1", "Local"
					}
					if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{container: map[string]any{"totalCount": 1, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{parent}}}}); err != nil {
						t.Error(err)
					}
				})
				var fields []value.FieldDescription
				var complete bool
				var err error
				if embedded {
					got, readErr := c.WorkbookDatasources(t.Context(), "wb-1")
					fields, complete, err = got.Datasources[0].Fields, got.Complete, readErr
				} else {
					got, readErr := c.datasourceFieldDescriptions(t.Context(), "ds-1", nil, true)
					fields, complete, err = got.Fields, got.Complete, readErr
				}
				if err == nil || complete || continuations != 1 || len(fields) != 2 {
					t.Fatalf("fields %+v, complete %v, continuations %d, error %v", fields, complete, continuations, err)
				}
				for i, suffix := range []string{"a", "b"} {
					field := fields[i]
					if field.MetadataID != "field-"+suffix || field.Description == nil || *field.Description != "Meaning "+suffix || len(field.UpstreamColumns) != 1 || field.UpstreamColumns[0].MetadataID != "column-"+suffix || len(field.Sheets) != 1 || field.Sheets[0].MetadataID != "sheet-"+suffix || len(field.ReferencedFields) != 1 || field.ReferencedFields[0].MetadataID != "reference-"+suffix {
						t.Fatalf("confirmed inline evidence lost: %+v", field)
					}
				}
			})
		}
	}
}
