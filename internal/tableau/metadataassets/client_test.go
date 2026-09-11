package metadataassets

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

type testSession struct{}

func (testSession) Authorize(r *http.Request) { r.Header.Set("X-Tableau-Auth", "fixture-token") }
func (testSession) SiteLUID() string          { return "site-1" }
func (testSession) UserLUID() string          { return "user-1" }
func (testSession) String() string            { return "session" }

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewTLSServer(handler)
	t.Cleanup(s.Close)
	return NewClient(tableau.NewTransport(s.Client(), "3.29", nil), testSession{}, s.URL)
}

func TestDatabaseUpdateOnlyRequestedProperty(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.Path != "/api/3.29/sites/site-1/databases/db-1" || string(body) != `<tsRequest><database description="Sales &amp; costs"></database></tsRequest>` {
			t.Fatalf("request %s %s %s", r.Method, r.URL, string(body))
		}
		io.WriteString(w, `<tsResponse><database id="db-1" name="Sales" type="DatabaseServer" description="Sales &amp; costs"><site id="site-1"/></database></tsResponse>`)
	})
	description := "Sales & costs"
	got, err := c.UpdateDatabase(context.Background(), "db-1", Update{Description: &description})
	if err != nil || got.LUID != "db-1" || got.Description == nil || *got.Description != description {
		t.Fatalf("got %#v %v", got, err)
	}
}

func TestGetLabelsIsReadOnlyPOST(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/api/3.29/sites/site-1/labels" || !strings.Contains(string(b), `contentType="table" id="table-1"`) {
			t.Fatalf("%s %s %s", r.Method, r.URL, b)
		}
		io.WriteString(w, `<tsResponse><labelList><label id="label-1" contentId="table-1" contentType="table" value="warning" category="warning" active="true" elevated="false"/></labelList></tsResponse>`)
	})
	got, err := c.GetLabels(context.Background(), LabelTarget{Type: "table", LUID: "table-1"}, nil)
	if err != nil || len(got) != 1 || got[0].LUID != "label-1" {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestGraphQLExactFilterAndBoundedPage(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		filter := body.Variables["filter"].(map[string]any)
		if filter["luid"] != "table-1" || body.Variables["first"] != float64(2) || !strings.Contains(body.Query, "databaseTablesConnection") {
			t.Fatalf("%#v", body)
		}
		io.WriteString(w, `{"data":{"items":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"id":"meta-1","luid":"table-1","name":"Sales","database":{"id":"db-meta","luid":"db-1","name":"Warehouse"}}]}}}`)
	})
	got, err := c.DiscoverTables(context.Background(), Query{LUID: "table-1", Limit: 2})
	if err != nil || !got.Complete || len(got.Items) != 1 || got.Items[0].MetadataID != "meta-1" {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestRejectsInvalidBeforeNetwork(t *testing.T) {
	c := fixtureClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected request") })
	empty := ""
	checks := []func() error{
		func() error {
			_, e := c.UpdateDatabase(context.Background(), "db", Update{ContactLUID: &empty})
			return e
		},
		func() error {
			_, e := c.UpdateColumn(context.Background(), "table", "column", Update{Description: &empty})
			return e
		},
		func() error { _, e := c.DiscoverColumns(context.Background(), Query{Text: "sales"}); return e },
		func() error {
			_, e := c.SetLabel(context.Background(), LabelTarget{Type: "workbook", LUID: "wb"}, LabelUpdate{Value: "warning"})
			return e
		},
		func() error {
			_, e := c.SetLabelValue(context.Background(), "", value.LabelValue{Name: "x", Category: "warning"})
			return e
		},
	}
	for i, check := range checks {
		if err := check(); err == nil {
			t.Fatalf("check %d accepted", i)
		}
	}
}

func TestMalformedSuccessfulIdentitiesFail(t *testing.T) {
	for _, body := range []string{`<tsResponse><database name="Sales"/></tsResponse>`, `<tsResponse><database id="other" name="Sales"/></tsResponse>`, `<tsResponse><database id="db-1" name="Sales"><site id="other"/></database></tsResponse>`, `<tsResponse><database id="db-1" name="Sales"/><database id="db-1" name="Sales"/></tsResponse>`} {
		t.Run(body, func(t *testing.T) {
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
			if _, err := c.GetDatabase(context.Background(), "db-1"); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
}
