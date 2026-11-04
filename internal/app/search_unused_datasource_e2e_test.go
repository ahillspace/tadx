package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestDefaultSearchResolvesOnlyDatasourcesNeededByOutputBudget(t *testing.T) {
	for _, scenario := range []struct {
		name                string
		secondPageMetrics   int
		limit               int
		wantResolverFailure bool
	}{
		{"unused datasource after extra metric", 2, 101, false},
		{"unused datasource immediately after budget", 1, 101, false},
		{"unused datasource after exact metric budget", 2, 102, false},
		{"required datasource at102", 1, 102, true},
		{"required datasource at103", 2, 103, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var nativeCalls, resolverCalls, unexpectedCalls atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/datasources" {
					resolverCalls.Add(1)
					if r.URL.Query().Get("filter") != "contentUrl:eq:sales" {
						t.Errorf("datasource resolution expanded beyond an exact content URL: %s", r.URL.RawQuery)
					}
					w.Header().Set("Retry-After", "15")
					w.Header().Set("X-Tableau-Request-Id", "datasource-resolution-throttled")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"message":"Too Many Requests"}`)
					return
				}
				if r.Method != http.MethodGet || r.URL.Path != "/api/-/search" {
					unexpectedCalls.Add(1)
					t.Errorf("default search traversed an unexpected source: %s %s", r.Method, r.URL)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				page := int(nativeCalls.Add(1)) - 1
				query := r.URL.Query()
				if page > 1 || query.Get("page") != strconv.Itoa(page) || query.Get("limit") != "100" || query.Get("terms") != "test" || query.Get("filter") != "type:in:[datasource,flow,insightmetric,project,workbook]" {
					t.Errorf("native request %d has unexpected paging or filters: %s", page+1, r.URL.RawQuery)
					http.Error(w, "unexpected query", http.StatusBadRequest)
					return
				}
				start, end := 0, 100
				next := "/api/-/search?page=1"
				if page == 1 {
					start, end = 100, 100+scenario.secondPageMetrics
					next = ""
				}
				items := make([]any, 0, end-start+1)
				for index := start; index < end; index++ {
					items = append(items, map[string]any{"uri": fmt.Sprintf("/metrics/index-%03d", index), "content": map[string]any{"type": "insightmetric", "metricId": fmt.Sprintf("metric-%03d", index), "title": fmt.Sprintf("test Metric %03d", index)}})
				}
				if page == 1 {
					items = append(items, map[string]any{"uri": "/datasources/connection-1", "content": map[string]any{"type": "datasource", "luid": "connection-1", "name": "test Sales connection", "repositoryUrl": "sales", "datasourceIsPublished": true, "parentType": "Datasource", "parentLuid": "datasource-1", "parentName": "test Sales"}})
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"items": items, "limit": 100, "pageIndex": page, "startIndex": page * 100, "total": 101 + scenario.secondPageMetrics, "next": next}); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			var output strings.Builder
			code := app.Run(t.Context(), []string{"search", "test", "--limit", strconv.Itoa(scenario.limit), "--environment", "test", "--json"}, &output, cloudPulseSearchOptions(t, server))
			var result struct {
				Items []struct {
					LUID string `json:"luid"`
					Type string `json:"type"`
					Name string `json:"name"`
				} `json:"items"`
				Page struct {
					Returned      int  `json:"returned"`
					MoreAvailable bool `json:"more_available"`
				} `json:"page"`
				Error struct {
					ID               string `json:"id"`
					Kind             string `json:"kind"`
					UpstreamStatus   int    `json:"upstream_status"`
					TableauRequestID string `json:"tableau_request_id"`
					Retryable        *bool  `json:"retryable"`
					CorrectiveAction string `json:"corrective_action"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
				t.Fatalf("decode CLI output: %v; output=%s", err, output.String())
			}
			if nativeCalls.Load() != 2 || unexpectedCalls.Load() != 0 {
				t.Fatalf("unexpected source traversal: native=%d unexpected=%d", nativeCalls.Load(), unexpectedCalls.Load())
			}
			if scenario.wantResolverFailure {
				if code == 0 || resolverCalls.Load() != 1 || len(result.Items) != 0 || result.Page.Returned != 0 || result.Error.ID != "search.operation" || result.Error.Kind != "operation" || result.Error.UpstreamStatus != 429 || result.Error.TableauRequestID != "datasource-resolution-throttled" || result.Error.Retryable == nil || !*result.Error.Retryable || !strings.Contains(result.Error.CorrectiveAction, "Wait") {
					t.Fatalf("required datasource failure lost its metadata: code=%d resolver-calls=%d output=%s", code, resolverCalls.Load(), output.String())
				}
				return
			}
			if code != 0 || resolverCalls.Load() != 0 || result.Error.ID != "" || len(result.Items) != scenario.limit || result.Page.Returned != scenario.limit || !result.Page.MoreAvailable {
				t.Fatalf("available metric prefix depended on an unused datasource: code=%d resolver-calls=%d output=%s", code, resolverCalls.Load(), output.String())
			}
			for index, item := range result.Items {
				if item.Type != "metric" || item.LUID != fmt.Sprintf("metric-%03d", index) || item.Name != fmt.Sprintf("test Metric %03d", index) {
					t.Fatalf("metric rank/identity changed at %d: %+v", index, item)
				}
			}
		})
	}
}
