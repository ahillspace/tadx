package search_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
)

type session struct{}

func (session) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", "token") }
func (session) SiteLUID() string                { return "site-1" }
func (session) UserLUID() string                { return "user-1" }
func (session) String() string                  { return "session" }

func newClient(t *testing.T, handler http.Handler, apiVersion string) *tableausearch.Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client, err := tableauSearchClient(tableau.NewTransport(server.Client(), apiVersion, nil), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func tableauSearchClient(transport *tableau.Transport, serverURL string) (*tableausearch.Client, error) {
	return tableauSearchClientWithSession(transport, session{}, serverURL)
}

func tableauSearchClientWithSession(transport *tableau.Transport, authenticated session, serverURL string) (*tableausearch.Client, error) {
	return tableausearch.NewClient(transport, authenticated, serverURL)
}

func TestSearchUsesNativeEndpointAndCanonicalTypeFilter(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/-/search" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("terms") != "Sales" || query.Get("filter") != "type:in:[datasource,workbook]" || query.Get("limit") != "2" || query.Get("page") != "1" {
			t.Fatalf("query=%q", request.URL.RawQuery)
		}
		if request.Header.Get("Accept") != "application/vnd.tableau.search-results.v2+json" || request.Header.Get("X-Tableau-Site-Id") != "site-1" || request.Header.Get("X-Tableau-Auth") != "token" {
			t.Fatalf("headers=%v", request.Header)
		}
		writer.Header().Set("X-Tableau-Request-Id", "search-request")
		_, _ = io.WriteString(writer, `{
			"items":[
				{"uri":"/workbooks/wb-1","content":{"luid":"wb-1","contentType":"WORKBOOK","name":" Sales Workbook ","project":{"luid":"project-1","name":"Sales"},"owner":{"luid":"owner-1","name":"Analyst"},"updatedAt":"2026-09-05T01:02:03Z"}},
				{"uri":"/datasources/ds-1","content":{"luid":"ds-1","type":"datasource","title":"Sales Source","projectLuid":"project-1","projectPath":"Department/Sales","ownerId":123,"ownerName":"Analyst","modifiedTime":"2026-09-04T01:02:03Z"}}
			],
			"limit":2,"pageIndex":1,"startIndex":2,"total":5,"next":"/api/-/search?page=2","prev":"/api/-/search?page=0"
		}`)
	}), "3.29")

	page, err := client.Search(context.Background(), tableausearch.Request{Terms: " Sales ", Types: []string{"WORKBOOK", "datasource", "workbook"}, Limit: 2, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.PageIndex != 1 || page.StartIndex != 2 || page.Total != 5 || !page.HasNext || page.TableauRequestID != "search-request" {
		t.Fatalf("page=%+v", page)
	}
	got := page.Items
	want := []tableausearch.Item{
		{LUID: "wb-1", Type: "workbook", Name: "Sales Workbook", ProjectLUID: "project-1", ProjectName: "Sales", OwnerLUID: "owner-1", OwnerName: "Analyst", ModifiedAt: "2026-09-05T01:02:03Z", URI: "/workbooks/wb-1"},
		{LUID: "ds-1", Type: "datasource", Name: "Sales Source", ProjectLUID: "project-1", ProjectPath: "Department/Sales", OwnerName: "Analyst", ModifiedAt: "2026-09-04T01:02:03Z", URI: "/datasources/ds-1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items=%#v", got)
	}
}

func TestSearchUsesEqFilterForOneTypeAtAPI316(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.URL.Query().Get("filter"); got != "type:eq:flow" {
			t.Fatalf("filter=%q", got)
		}
		_, _ = io.WriteString(writer, `{"items":[],"limit":10,"pageIndex":0,"startIndex":0,"total":0}`)
	}), "3.16")
	if _, err := client.Search(context.Background(), tableausearch.Request{Types: []string{"flow"}, Limit: 10}); err != nil {
		t.Fatal(err)
	}
}

func TestSearchAcceptsCurrentWrappedHitsEnvelope(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, `{
			"terms":"sales","filter":"type:eq:workbook","personalized":true,"drafts":[],
			"hits":{"items":[{"uri":"/datasources/ds-1","score":1.5,"content":{"luid":"ds-1","type":"unifieddatasource","title":"Sales","containerType":"project","containerName":"Finance","ownerName":"Analyst"}}],"limit":1,"pageIndex":0,"startIndex":0,"total":1}
		}`)
	}), "3.29")
	page, err := client.Search(context.Background(), tableausearch.Request{Terms: "sales", Types: []string{"datasource"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].LUID != "ds-1" || page.Items[0].Type != "datasource" || page.Items[0].ProjectName != "Finance" || page.HasNext {
		t.Fatalf("page=%+v", page)
	}
}

func TestSearchAcceptsWrappedZeroResultsWithoutItems(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, `{"hits":{"limit":5,"pageIndex":0,"startIndex":0,"total":0}}`)
	}), "3.29")
	page, err := client.Search(context.Background(), tableausearch.Request{Terms: "missing", Types: []string{"workbook"}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Total != 0 || page.HasNext {
		t.Fatalf("page=%+v", page)
	}
}

func TestSearchRejectsUnsupportedInputsBeforeNetwork(t *testing.T) {
	calls := 0
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { calls++ }), "3.29")
	tests := []tableausearch.Request{
		{Terms: "sales", Types: []string{"user"}, Limit: 10},
		{Terms: "sales", Types: []string{"metric"}, Limit: 10},
		{Terms: "sales", Limit: 10},
		{Types: []string{"workbook"}, Limit: 0},
		{Types: []string{"workbook"}, Limit: 101},
		{Types: []string{"workbook"}, Limit: 100, Page: 20},
		{Types: []string{"workbook"}, Limit: 10, Page: -1},
	}
	for _, input := range tests {
		if _, err := client.Search(context.Background(), input); err == nil {
			t.Fatalf("accepted %#v", input)
		}
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestSearchReturnsAvailabilityErrorForOldOrMissingEndpoint(t *testing.T) {
	old := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatal("old API version reached the network")
	}), "3.15")
	_, err := old.Search(context.Background(), tableausearch.Request{Types: []string{"workbook"}, Limit: 10})
	var unavailable interface{ NativeSearchUnavailable() bool }
	if !errors.As(err, &unavailable) || !unavailable.NativeSearchUnavailable() {
		t.Fatalf("old version error=%v", err)
	}

	multi := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatal("API 3.16 multi-type request reached the network")
	}), "3.16")
	_, err = multi.Search(context.Background(), tableausearch.Request{Types: []string{"workbook", "flow"}, Limit: 10})
	if !errors.As(err, &unavailable) {
		t.Fatalf("multi-type error=%v", err)
	}

	missing := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "missing-request")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, `{"error":{"code":"404000","summary":"Not Found","detail":"Search is unavailable"}}`)
	}), "3.29")
	_, err = missing.Search(context.Background(), tableausearch.Request{Types: []string{"workbook"}, Limit: 10})
	var status interface{ HTTPStatus() int }
	if !errors.As(err, &unavailable) || !errors.As(err, &status) || status.HTTPStatus() != http.StatusNotFound || tableau.RequestID(err) != "missing-request" || !strings.Contains(err.Error(), "Search is unavailable") {
		t.Fatalf("missing endpoint error=%v requestID=%q", err, tableau.RequestID(err))
	}
}

func TestSearchPreservesStructuredUpstreamErrors(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "busy-request")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"error":{"code":"503000","summary":"Unavailable","detail":"Try later"}}`)
	}), "3.29")
	_, err := client.Search(context.Background(), tableausearch.Request{Terms: "sales", Types: []string{"workbook"}, Limit: 10})
	var status interface{ HTTPStatus() int }
	var retryable interface{ Retryable() bool }
	if !errors.As(err, &status) || status.HTTPStatus() != http.StatusServiceUnavailable || !errors.As(err, &retryable) || !retryable.Retryable() || tableau.RequestID(err) != "busy-request" {
		t.Fatalf("error=%v requestID=%q", err, tableau.RequestID(err))
	}
}

func TestSearchRejectsInvalidSuccessEnvelopeWithRequestContext(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "invalid-request")
		_, _ = io.WriteString(writer, `{"items":[{"content":{"luid":"wb-1","type":"workbook","name":"A"}},{"content":{"luid":"wb-1","type":"workbook","name":"B"}}],"limit":2,"pageIndex":0,"startIndex":0,"total":2}`)
	}), "3.29")
	_, err := client.Search(context.Background(), tableausearch.Request{Types: []string{"workbook"}, Limit: 2})
	var status interface{ HTTPStatus() int }
	if err == nil || tableau.RequestID(err) != "invalid-request" || !errors.As(err, &status) || status.HTTPStatus() != http.StatusOK {
		t.Fatalf("error=%v requestID=%q", err, tableau.RequestID(err))
	}
}

func TestSearchDoesNotTreatNumericSearchAssociationIDsAsLUIDs(t *testing.T) {
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, `{"items":[{"content":{"type":"workbook","title":"Sales","id":"numeric-search-id","projectId":123,"ownerId":456}}],"limit":1,"pageIndex":0,"startIndex":0,"total":1}`)
	}), "3.29")
	if _, err := client.Search(context.Background(), tableausearch.Request{Types: []string{"workbook"}, Limit: 1}); err == nil {
		t.Fatal("accepted a native search ID as a Tableau LUID")
	}
}
