package metadataassets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

type testSession struct{}

type verificationFailure interface {
	error
	VerificationFailed() bool
}

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

func TestGetLabelsNormalizesNativeDatasourceType(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || !strings.Contains(string(body), `contentType="datasource" id="datasource-1"`) {
			t.Fatalf("unexpected label request %s %s", r.Method, body)
		}
		io.WriteString(w, `<tsResponse><labelList><label id="label-1" contentId="datasource-1" contentType="DATASOURCE" value="warning" category="warning" active="true" elevated="false"/></labelList></tsResponse>`)
	})
	got, err := c.GetLabels(context.Background(), LabelTarget{Type: "datasource", LUID: "datasource-1"}, nil)
	if err != nil || len(got) != 1 || got[0].Type != "datasource" {
		t.Fatalf("native datasource type was not normalized: %#v %v", got, err)
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
			_, e := c.UpdateDatabase(context.Background(), "db", Update{Description: &empty})
			return e
		},
		func() error {
			_, e := c.UpdateTable(context.Background(), "table", Update{Description: &empty})
			return e
		},
		func() error {
			_, e := c.UpdateColumn(context.Background(), "table", "column", Update{Description: &empty, ContactLUID: &empty})
			return e
		},
		func() error {
			_, e := c.UpdateColumn(context.Background(), "table", "column", Update{})
			return e
		},
		func() error {
			oversized := strings.Repeat("x", 65537)
			_, e := c.UpdateColumn(context.Background(), "table", "column", Update{Description: &oversized})
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

func TestColumnDescriptionClearSendsExplicitEmptyProperty(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.Method != "PUT" || r.URL.Path != "/api/3.29/sites/site-1/tables/table-1/columns/column-1" || string(body) != `<tsRequest><column description=""></column></tsRequest>` {
			t.Fatalf("unexpected clearing request %s %s %s", r.Method, r.URL, body)
		}
		io.WriteString(w, `<tsResponse><column id="column-1" parentTableId="table-1" name="Category" description="" remoteType="string"><tags><tag label="keep"/></tags></column></tsResponse>`)
	})
	empty := ""
	got, err := c.UpdateColumn(context.Background(), "table-1", "column-1", Update{Description: &empty})
	if err != nil || calls != 1 || got.LUID != "column-1" || got.Table.LUID != "table-1" || got.Description == nil || *got.Description != "" || got.RemoteType != "string" || len(got.Tags) != 1 || got.Tags[0] != "keep" {
		t.Fatalf("got=%+v err=%v calls=%d", got, err, calls)
	}
}

func TestColumnClearRequiresVerifiedReadback(t *testing.T) {
	for _, description := range []string{`description="Old meaning"`, ""} {
		t.Run(description, func(t *testing.T) {
			calls := 0
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("X-Tableau-Request-Id", "clear-readback-request")
				io.WriteString(w, `<tsResponse><column id="column-1" parentTableId="table-1" name="Category" `+description+`/></tsResponse>`)
			})
			empty := ""
			got, err := c.UpdateColumn(context.Background(), "table-1", "column-1", Update{Description: &empty})
			protocol, protocolOK := errors.AsType[*tableau.ProtocolError](err)
			verification, verificationOK := errors.AsType[verificationFailure](err)
			if !protocolOK || protocol.RequestID() != "clear-readback-request" || !verificationOK || !verification.VerificationFailed() || got.LUID != "column-1" || calls != 1 {
				t.Fatalf("unverified clear lost identity or was accepted: got=%+v err=%v calls=%d", got, err, calls)
			}
		})
	}
}

func TestContactReadbackFailurePreservesVerificationDetails(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Tableau-Request-Id", "contact-readback-request")
		io.WriteString(w, `<tsResponse><database id="db-1" name="Sales"><contact id="user-1"/></database></tsResponse>`)
	})
	contact := "user-2"
	got, err := c.UpdateDatabase(t.Context(), "db-1", Update{ContactLUID: &contact})
	protocol, protocolOK := errors.AsType[*tableau.ProtocolError](err)
	verification, verificationOK := errors.AsType[verificationFailure](err)
	if !protocolOK || protocol.RequestID() != "contact-readback-request" || !verificationOK || !verification.VerificationFailed() || got.LUID != "db-1" {
		t.Fatalf("unverified contact lost identity or protocol details: got=%+v err=%v", got, err)
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
