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

func TestSearchDoesNotFetchNativePageAfterSatisfyingBudget(t *testing.T) {
	for _, scenario := range []struct {
		selector, filter, kind, providerKind, title string
	}{
		{"metric", "type:eq:insightmetric", "metric", "insightmetric", "Metric"},
		{"pulse", "type:eq:insightmetric", "metric", "insightmetric", "Metric"},
		{"", "type:in:[datasource,flow,insightmetric,project,workbook]", "metric", "insightmetric", "Metric"},
		{"content", "type:in:[datasource,flow,project,workbook]", "workbook", "workbook", "Workbook"},
		{"workbook", "type:eq:workbook", "workbook", "workbook", "Workbook"},
	} {
		for _, limit := range []int{20, 100, 101} {
			t.Run(fmt.Sprintf("selector=%s/limit%d", scenario.selector, limit), func(t *testing.T) {
				var nativeCalls, unexpectedCalls atomic.Int32
				pageSize := min(limit, 100)
				server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if diagnosticSignIn(w, r) {
						return
					}
					if r.Method != http.MethodGet || r.URL.Path != "/api/-/search" {
						unexpectedCalls.Add(1)
						t.Errorf("search traversed an unexpected source: %s %s", r.Method, r.URL)
						http.Error(w, "unexpected source", http.StatusBadRequest)
						return
					}
					page := int(nativeCalls.Add(1)) - 1
					query := r.URL.Query()
					if page > 1 || query.Get("terms") != "test" || query.Get("filter") != scenario.filter || query.Get("limit") != strconv.Itoa(pageSize) || query.Get("page") != strconv.Itoa(page) {
						t.Errorf("native request %d has unexpected paging or filters: %s", page+1, r.URL.RawQuery)
						http.Error(w, "unexpected native query", http.StatusBadRequest)
						return
					}
					if page == 1 {
						w.Header().Set("Retry-After", "15")
						w.Header().Set("X-Tableau-Request-Id", "native-next-page-throttled")
						w.WriteHeader(http.StatusTooManyRequests)
						_, _ = io.WriteString(w, `{"message":"Too Many Requests"}`)
						return
					}
					items := make([]any, pageSize)
					for index := range pageSize {
						content := map[string]any{"type": scenario.providerKind, "title": fmt.Sprintf("test %s %04d", scenario.title, index)}
						idKey := "luid"
						if scenario.kind == "metric" {
							idKey = "metricId"
						}
						content[idKey] = fmt.Sprintf("%s-%04d", scenario.kind, index)
						items[index] = map[string]any{"uri": fmt.Sprintf("/%ss/index-%04d", scenario.kind, index), "content": content}
					}
					w.Header().Set("X-Tableau-Request-Id", "native-first-page")
					if err := json.NewEncoder(w).Encode(map[string]any{"items": items, "limit": pageSize, "pageIndex": 0, "startIndex": 0, "total": pageSize + 1, "next": "/api/-/search?page=1"}); err != nil {
						t.Error(err)
					}
				}))
				t.Cleanup(server.Close)
				var output strings.Builder
				args := []string{"search", "test", "--limit", strconv.Itoa(limit), "--environment", "test", "--json"}
				if scenario.selector != "" {
					args = append(args, "--type", scenario.selector)
				}
				code := app.Run(t.Context(), args, &output, cloudPulseSearchOptions(t, server))
				var result struct {
					Items []struct {
						LUID string `json:"luid"`
						Type string `json:"type"`
						Name string `json:"name"`
					} `json:"items"`
					Page struct {
						Limit         int  `json:"limit"`
						Returned      int  `json:"returned"`
						MoreAvailable bool `json:"more_available"`
					} `json:"page"`
					Error struct {
						ID               string `json:"id"`
						UpstreamStatus   int    `json:"upstream_status"`
						TableauRequestID string `json:"tableau_request_id"`
						Retryable        *bool  `json:"retryable"`
						CorrectiveAction string `json:"corrective_action"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
					t.Fatalf("decode CLI output: %v; output=%s", err, output.String())
				}
				if unexpectedCalls.Load() != 0 {
					t.Fatalf("search called %d definition/admin/metric-inventory or other unexpected routes", unexpectedCalls.Load())
				}
				if limit == 101 {
					if code == 0 || nativeCalls.Load() != 2 || len(result.Items) != 0 || result.Page.Returned != 0 || result.Error.ID != "search.operation" || result.Error.UpstreamStatus != 429 || result.Error.TableauRequestID != "native-next-page-throttled" || result.Error.Retryable == nil || !*result.Error.Retryable || !strings.Contains(result.Error.CorrectiveAction, "Wait") {
						t.Fatalf("needed second native page failure lost its metadata: code=%d native-calls=%d output=%s", code, nativeCalls.Load(), output.String())
					}
					return
				}
				if code != 0 || nativeCalls.Load() != 1 || result.Error.ID != "" || result.Page.Limit != limit || result.Page.Returned != limit || len(result.Items) != limit || !result.Page.MoreAvailable {
					t.Fatalf("satisfied native budget fetched another page or lost results: code=%d native-calls=%d output=%s", code, nativeCalls.Load(), output.String())
				}
				for index, item := range result.Items {
					if item.Type != scenario.kind || item.LUID != fmt.Sprintf("%s-%04d", scenario.kind, index) || item.Name != fmt.Sprintf("test %s %04d", scenario.title, index) {
						t.Fatalf("native rank/identity changed at %d: %+v", index, item)
					}
				}
			})
		}
	}
}
