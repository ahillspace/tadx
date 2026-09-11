package metadataassets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

func TestAcknowledgedWritesPreserveProofWithoutInventingIdentity(t *testing.T) {
	for _, kind := range []string{"label-create", "label-update", "value", "category"} {
		t.Run(kind, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Tableau-Request-Id", "ack-request")
				io.WriteString(w, `<tsResponse><broken`)
			})
			var e error
			switch kind {
			case "label-create":
				v, err := c.SetLabel(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, LabelUpdate{Value: "warning"})
				e = err
				if v.LUID != "" || v.TargetLUID != "table-1" {
					t.Fatalf("invented attachment %#v", v)
				}
			case "label-update":
				v, err := c.UpdateLabel(context.Background(), "label-1", LabelUpdate{Value: "warning"})
				e = err
				if v.LUID != "label-1" {
					t.Fatalf("lost known attachment %#v", v)
				}
			case "value":
				v, err := c.SetLabelValue(context.Background(), "", value.LabelValue{Name: "Review", Category: "custom", Description: "Review guidance"})
				e = err
				if v.Name != "Review" {
					t.Fatalf("lost named target %#v", v)
				}
			case "category":
				v, err := c.CreateLabelCategory(context.Background(), value.LabelCategory{Name: "Review", Description: "Review guidance"})
				e = err
				if v.Name != "Review" {
					t.Fatalf("lost named target %#v", v)
				}
			}
			var classified interface{ WriteAcknowledged() bool }
			if !errors.As(e, &classified) || !classified.WriteAcknowledged() {
				t.Fatalf("incorrect acknowledgement %v", e)
			}
			var protocolError *tableau.ProtocolError
			if !errors.As(e, &protocolError) {
				t.Fatalf("lost protocol evidence %v", e)
			}
		})
	}
}

func TestRejectedOrUnreadWritesDoNotClaimAcknowledgement(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		io.WriteString(w, `<tsResponse><error code="503000"><summary>Unavailable</summary></error></tsResponse>`)
	})
	got, e := c.SetLabel(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, LabelUpdate{Value: "warning"})
	var classified interface{ WriteAcknowledged() bool }
	if e == nil || got.LUID != "" || (errors.As(e, &classified) && classified.WriteAcknowledged()) {
		t.Fatalf("false acknowledgement %#v %v", got, e)
	}
}

func TestMismatchedNewLabelResponseDoesNotLeakAttachmentIdentity(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<tsResponse><labelList><label id="unrelated-label" contentId="other-table" contentType="table" value="warning" category="warning" active="false" elevated="false"/></labelList></tsResponse>`)
	})
	got, e := c.SetLabel(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, LabelUpdate{Value: "warning"})
	var classified interface{ WriteAcknowledged() bool }
	if !errors.As(e, &classified) || !classified.WriteAcknowledged() || got.LUID != "" || got.TargetLUID != "table-1" {
		t.Fatalf("mismatchedidentity escaped %#v %v", got, e)
	}
}

func TestGraphQLRejectsIncompleteCoverageAndConflicts(t *testing.T) {
	for name, body := range map[string]string{
		"warning":       `{"data":{"items":{"nodes":[]}},"warnings":[{"message":"partial"}]}`,
		"error":         `{"errors":[{"message":"denied"}],"data":null}`,
		"nullnodes":     `{"data":{"items":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":null}}}`,
		"missingtotal":  `{"data":{"items":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}`,
		"stalled":       `{"data":{"items":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"again"},"nodes":[{"id":"meta","luid":"db","name":"Sales"}]}}}`,
		"emptyidentity": `{"data":{"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"name":"Sales"}]}}}`,
		"conflicting":   `{"data":{"items":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta","luid":"db","name":"Sales"},{"id":"meta","luid":"other","name":"Sales"}]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Tableau-Request-Id", "request-1")
				io.WriteString(w, body)
			})
			_, err := c.DiscoverDatabases(context.Background(), Query{Cursor: "again"})
			if err == nil {
				t.Fatal("accepted incomplete evidence")
			}
		})
	}
}

func TestParentScopedColumnsUseNestedConnection(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Query     string
			Variables map[string]any
		}
		json.NewDecoder(r.Body).Decode(&b)
		if b.Variables["parent"] != "table-1" || !strings.Contains(b.Query, "parents:databaseTablesConnection") || !strings.Contains(b.Query, "items:columnsConnection") {
			t.Fatalf("%#v", b)
		}
		io.WriteString(w, `{"data":{"parents":{"nodes":[{"luid":"table-1","items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"meta-c","luid":"col-1","name":"Revenue","table":{"id":"meta-t","luid":"table-1","name":"Orders"}}]}}]}}}`)
	})
	got, e := c.DiscoverColumns(context.Background(), Query{ParentLUID: "table-1"})
	if e != nil || len(got.Items) != 1 || got.Items[0].Table.LUID != "table-1" {
		t.Fatalf("%#v %v", got, e)
	}
}

func TestMetadataOnlyIdentityRemainsReadable(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"metadata-only","name":"External source","__typename":"File"}]}}}`)
	})
	got, e := c.DiscoverDatabases(context.Background(), Query{MetadataID: "metadata-only"})
	if e != nil || len(got.Items) != 1 || got.Items[0].LUID != "" || got.Items[0].MetadataID != "metadata-only" {
		t.Fatalf("%#v %v", got, e)
	}
}

func TestOldRESTVersionStopsBeforeRequest(t *testing.T) {
	c := fixtureClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("old version sent request") })
	c.transport = tableau.NewTransport(http.DefaultClient, "3.15", nil)
	if _, e := c.GetLabels(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, nil); e == nil {
		t.Fatal("unverified old label contract accepted")
	}
}

func TestLabelAndVocabularyWireContracts(t *testing.T) {
	labelResponse := `<label id="label-1" contentId="table-1" contentType="table" value="warning" category="warning" message="Watch" active="false" elevated="true"/>`
	valueResponse := `<labelValue name="Review / approved" category="custom" description="Reviewed" internal="false" elevatedDefault="false" builtIn="false"/>`
	categoryResponse := `<labelCategory name="Custom / review" description="Reviewed"/>`
	for _, tc := range []struct {
		name, method, path, body, response string
		call                               func(*Client) error
	}{
		{"get-label", "GET", "labels/label-1", "", labelResponse, func(c *Client) error { _, e := c.GetLabel(context.Background(), "label-1"); return e }},
		{"set-label", "PUT", "labels", `<contentList><content contentType="table" id="table-1"></content></contentList>`, `<labelList>` + labelResponse + `</labelList>`, func(c *Client) error {
			_, e := c.SetLabel(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, LabelUpdate{Value: "warning", Message: "Watch", Elevated: true})
			return e
		}},
		{"update-label", "PUT", "labels/label-1", `active="false" elevated="true"`, labelResponse, func(c *Client) error {
			_, e := c.UpdateLabel(context.Background(), "label-1", LabelUpdate{Value: "warning", Message: "Watch", Elevated: true})
			return e
		}},
		{"delete-label", "DELETE", "labels/label-1", "", "", func(c *Client) error { return c.DeleteLabel(context.Background(), "label-1") }},
		{"values", "GET", "labelValues", "", `<labelValueList>` + valueResponse + `</labelValueList>`, func(c *Client) error { _, e := c.ListLabelValues(context.Background()); return e }},
		{"value-inspect", "GET", "labelValues/Review%20%2F%20approved", "", valueResponse, func(c *Client) error { _, e := c.GetLabelValue(context.Background(), "Review / approved"); return e }},
		{"value-upsert", "PUT", "labelValues", `<labelValue name="Review / approved" category="custom" description="Reviewed">`, valueResponse, func(c *Client) error {
			_, e := c.SetLabelValue(context.Background(), "", value.LabelValue{Name: "Review / approved", Category: "custom", Description: "Reviewed"})
			return e
		}},
		{"value-rename", "PUT", "labelValues/Old", `<labelValue name="Review / approved"`, valueResponse, func(c *Client) error {
			_, e := c.SetLabelValue(context.Background(), "Old", value.LabelValue{Name: "Review / approved", Category: "custom", Description: "Reviewed"})
			return e
		}},
		{"value-delete", "DELETE", "labelValues/Old", "", "", func(c *Client) error { return c.DeleteLabelValue(context.Background(), "Old") }},
		{"category-inspect", "GET", "labelCategories", "", `<labelCategoryList>` + categoryResponse + `</labelCategoryList>`, func(c *Client) error { _, e := c.GetLabelCategory(context.Background(), "Custom / review"); return e }},
		{"category-create", "POST", "labelCategories", `<labelCategory name="Custom / review" description="Reviewed">`, categoryResponse, func(c *Client) error {
			_, e := c.CreateLabelCategory(context.Background(), value.LabelCategory{Name: "Custom / review", Description: "Reviewed"})
			return e
		}},
		{"category-update", "PUT", "labelCategories/Old", `<labelCategory name="Custom / review" description="Reviewed">`, categoryResponse, func(c *Client) error {
			_, e := c.UpdateLabelCategory(context.Background(), "Old", value.LabelCategory{Name: "Custom / review", Description: "Reviewed"})
			return e
		}},
		{"category-delete", "DELETE", "labelCategories/Old", "", "", func(c *Client) error { return c.DeleteLabelCategory(context.Background(), "Old") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				if r.Method != tc.method || r.URL.EscapedPath() != "/api/3.29/sites/site-1/"+tc.path || !strings.Contains(string(b), tc.body) {
					t.Fatalf("%s %s %s", r.Method, r.URL, b)
				}
				if tc.response == "" {
					if strings.HasPrefix(tc.path, "labels/") {
						w.WriteHeader(204)
					}
					return
				}
				io.WriteString(w, "<tsResponse>"+tc.response+"</tsResponse>")
			})
			if e := tc.call(c); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestMutationPreservesIdentityAfterReadbackFailure(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Tableau-Request-Id", "request-1")
		io.WriteString(w, `<tsResponse><table id="table-1" name="Orders" description="Old"/></tsResponse>`)
	})
	description := "New"
	got, e := c.UpdateTable(context.Background(), "table-1", Update{Description: &description})
	var p *tableau.ProtocolError
	if e == nil || got.LUID != "table-1" || !errors.As(e, &p) {
		t.Fatalf("%#v %v", got, e)
	}
}

func TestDatasourceFieldProvenancePaginatesColumnsIndependently(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var b struct {
			Query     string
			Variables map[string]any
		}
		json.NewDecoder(r.Body).Decode(&b)
		if calls == 1 {
			if !strings.Contains(b.Query, "descriptionInherited") || !strings.Contains(b.Query, "tagsConnection") {
				t.Fatal("query omitted provenance")
			}
			io.WriteString(w, `{"data":{"parents":{"nodes":[{"id":"ds-meta","luid":"ds-1","name":"Sales","description":"Sales data","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"field-meta","name":"Revenue","fullyQualifiedName":"[raw_revenue]","description":"Published description","descriptionInherited":[{"assetId":"column-meta","attribute":"description","value":"Physical description","distance":1,"edges":[]}],"upstreamColumnsConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"col-next"},"nodes":[{"id":"column-meta","luid":"col-1","name":"raw_revenue","description":"Physical description"}]}}]}}]}}}`)
		} else {
			if b.Variables["field"] != "field-meta" || b.Variables["after"] != "col-next" {
				t.Fatalf("%#v", b)
			}
			io.WriteString(w, `{"data":{"parents":{"nodes":[{"luid":"ds-1","items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"field-meta","upstreamColumnsConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"column-meta2","luid":"col-2","name":"raw_revenue2","description":null}]}}]}}]}}}`)
		}
	})
	v, e := c.DatasourceFieldDescriptions(context.Background(), "ds-1")
	if e != nil || calls != 2 || !v.Complete || !v.TagsObserved || len(v.Fields) != 1 || !v.Fields[0].InheritedObserved || len(v.Fields[0].UpstreamColumns) != 2 || v.Fields[0].FullyQualifiedName != "[raw_revenue]" || v.Fields[0].UpstreamColumns[1].Description != nil {
		t.Fatalf("%#v calls%d %v", v, calls, e)
	}
}

func TestTagsAndContactWireContracts(t *testing.T) {
	t.Run("tags", func(t *testing.T) {
		c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if r.Method != "PUT" || r.URL.Path != "/api/3.29/sites/site-1/columns/col-1/tags" || !strings.Contains(string(b), `label="sales &amp; retail"`) {
				t.Fatalf("%s %s %s", r.Method, r.URL, b)
			}
			io.WriteString(w, `<tsResponse><tags><tag label="sales &amp; retail"/></tags></tsResponse>`)
		})
		if _, e := c.AddTags(context.Background(), LabelTarget{Type: "column", LUID: "col-1"}, []string{"sales & retail"}); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("contact", func(t *testing.T) {
		c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if strings.Contains(string(b), "description") || !strings.Contains(string(b), `<contact id="user-1">`) {
				t.Fatal(string(b))
			}
			io.WriteString(w, `<tsResponse><database id="db-1" name="Sales"><contact id="user-1"/></database></tsResponse>`)
		})
		id := "user-1"
		if _, e := c.UpdateDatabase(context.Background(), "db-1", Update{ContactLUID: &id}); e != nil {
			t.Fatal(e)
		}
	})
}
