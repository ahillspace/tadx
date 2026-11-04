package search_test

import (
	"io"
	"net/http"
	"reflect"
	"testing"

	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
)

func TestSearchMetricUsesIndexedSearchAndAuthoritativeMetricIdentity(t *testing.T) {
	calls := 0
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		query := request.URL.Query()
		if request.Method != http.MethodGet || request.URL.Path != "/api/-/search" || query.Get("filter") != "type:eq:insightmetric" || query.Get("terms") != "sales" || query.Get("limit") != "1" || query.Get("page") != "0" {
			t.Errorf("request=%s %s", request.Method, request.URL)
		}
		_, _ = io.WriteString(writer, `{"hits":{"items":[{"uri":"/insightmetrics/index-entry","content":{"type":"insightmetric","id":"index-id","luid":"index-luid","metricId":"metric-1","definitionId":"definition-1","name":"Index name","title":"Sales"}}],"limit":1,"pageIndex":0,"startIndex":0,"total":2,"next":"/api/-/search?page=1"}}`)
	}), "3.29")
	page, err := client.Search(t.Context(), tableausearch.Request{Terms: "sales", Types: []string{"metric"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := []tableausearch.Item{{LUID: "metric-1", Type: "metric", Name: "Sales", URI: "/insightmetrics/index-entry"}}
	if calls != 1 || !reflect.DeepEqual(page.Items, want) || page.Total != 2 || !page.HasNext {
		t.Fatalf("calls=%d page=%+v", calls, page)
	}
}

func TestSearchMixesMetricsWithContentWithoutSeparateQueries(t *testing.T) {
	calls := 0
	client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if got := request.URL.Query().Get("filter"); got != "type:in:[datasource,insightmetric,workbook]" {
			t.Errorf("filter=%q", got)
		}
		_, _ = io.WriteString(writer, `{"items":[{"content":{"type":"insightmetric","metricId":"metric-1","title":"Sales"}},{"content":{"type":"workbook","luid":"workbook-1","name":"Sales dashboard"}}],"limit":2,"pageIndex":0,"startIndex":0,"total":2}`)
	}), "3.29")
	page, err := client.Search(t.Context(), tableausearch.Request{Terms: "sales", Types: []string{"workbook", "metric", "datasource", "METRIC"}, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := []tableausearch.Item{{LUID: "metric-1", Type: "metric", Name: "Sales"}, {LUID: "workbook-1", Type: "workbook", Name: "Sales dashboard"}}
	if calls != 1 || !reflect.DeepEqual(page.Items, want) || page.HasNext {
		t.Fatalf("calls=%d page=%+v", calls, page)
	}
}

func TestSearchMetricRejectsNonAuthoritativeOrDuplicateIdentity(t *testing.T) {
	for _, test := range []struct {
		name  string
		items string
		limit int
	}{
		{name: "missing metric ID", items: `[{"content":{"type":"insightmetric","id":"index-id","luid":"index-luid","title":"Sales"}}]`, limit: 1},
		{name: "blank metric ID", items: `[{"content":{"type":"insightmetric","metricId":" ","luid":"index-luid","title":"Sales"}}]`, limit: 1},
		{name: "missing title", items: `[{"content":{"type":"insightmetric","metricId":"metric-1","name":"Index name"}}]`, limit: 1},
		{name: "duplicate canonical ID", items: `[{"content":{"type":"insightmetric","metricId":"metric-1","luid":"index-1","title":"Sales"}},{"content":{"type":"insightmetric","metricId":"metric-1","luid":"index-2","title":"Sales"}}]`, limit: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = io.WriteString(writer, `{"items":`+test.items+`,"limit":`+request.URL.Query().Get("limit")+`,"pageIndex":0,"startIndex":0,"total":`+request.URL.Query().Get("limit")+`}`)
			}), "3.29")
			if _, err := client.Search(t.Context(), tableausearch.Request{Terms: "sales", Types: []string{"metric"}, Limit: test.limit}); err == nil {
				t.Fatal("accepted invalid metric identity")
			}
		})
	}
}
