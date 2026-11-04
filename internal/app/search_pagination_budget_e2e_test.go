package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

type pulsePaginationCalls struct {
	native           atomic.Int32
	definitions      atomic.Int32
	metricLists      atomic.Int32
	definitionStatus atomic.Int32
	generation       atomic.Int32
	mu               sync.Mutex
	definitionTokens []string
}

func pulsePaginationOptions(t *testing.T, nativeCount, definitionCount, definitionStatus int) (app.Options, *pulsePaginationCalls) {
	t.Helper()
	calls := &pulsePaginationCalls{}
	calls.definitionStatus.Store(int32(definitionStatus))
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		query := r.URL.Query()
		switch {
		case r.URL.Path == "/api/-/search":
			calls.native.Add(1)
			if query.Get("filter") != "type:eq:insightmetric" || query.Get("terms") != "test" {
				t.Errorf("unexpected native query: %s", r.URL.RawQuery)
				http.Error(w, "unexpected query", http.StatusBadRequest)
				return
			}
			limit, limitErr := strconv.Atoi(query.Get("limit"))
			page, pageErr := strconv.Atoi(query.Get("page"))
			if limitErr != nil || pageErr != nil || limit < 1 || limit > 100 || page < 0 || page*limit > nativeCount {
				t.Errorf("invalid native paging: %s", r.URL.RawQuery)
				http.Error(w, "invalid paging", http.StatusBadRequest)
				return
			}
			start, end := page*limit, min((page+1)*limit, nativeCount)
			items := []any{}
			for index := start; index < end; index++ {
				items = append(items, map[string]any{"uri": fmt.Sprintf("/metrics/index-%04d", index), "content": map[string]any{"type": "insightmetric", "metricId": fmt.Sprintf("metric-%04d", index), "title": fmt.Sprintf("test Metric %04d", index)}})
			}
			next := ""
			if end < nativeCount {
				next = fmt.Sprintf("/api/-/search?page=%d", page+1)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "limit": limit, "pageIndex": page, "startIndex": start, "total": nativeCount, "next": next})
		case r.URL.Path == "/api/-/pulse/definitions":
			calls.definitions.Add(1)
			calls.mu.Lock()
			calls.definitionTokens = append(calls.definitionTokens, query.Get("page_token"))
			calls.mu.Unlock()
			if query.Get("filter") != "name:has:test" || query.Get("page_size") != "100" {
				t.Errorf("unexpected definition query: %s", r.URL.RawQuery)
				http.Error(w, "unexpected query", http.StatusBadRequest)
				return
			}
			if status := calls.definitionStatus.Load(); status != 0 {
				w.Header().Set("Retry-After", "15")
				w.Header().Set("X-Tableau-Request-Id", "definitions-throttled")
				w.WriteHeader(int(status))
				_, _ = io.WriteString(w, `{"httpErrorCode":429,"message":"Too Many Requests"}`)
				return
			}
			start := 0
			if token := query.Get("page_token"); token != "" {
				value, ok := strings.CutPrefix(token, "offset-")
				var err error
				start, err = strconv.Atoi(value)
				if !ok || err != nil || start < 0 || start >= definitionCount {
					t.Errorf("invalid definition token: %q", token)
					http.Error(w, "invalid token", http.StatusBadRequest)
					return
				}
			}
			end := min(start+100, definitionCount)
			definitions := []any{}
			for index := start; index < end; index++ {
				definitions = append(definitions, map[string]any{"metadata": map[string]any{"id": fmt.Sprintf("definition-%04d", index), "name": fmt.Sprintf("test Definition %04d version %d", index, calls.generation.Load())}, "specification": map[string]any{"datasource": map[string]any{"id": "datasource-1"}}})
			}
			next := ""
			if end < definitionCount {
				next = fmt.Sprintf("offset-%d", end)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"definitions": definitions, "next_page_token": next})
		case strings.Contains(r.URL.Path, "/metrics"):
			calls.metricLists.Add(1)
			t.Errorf("term search traversed a metric inventory: %s", r.URL.Path)
			http.Error(w, "unexpected metric inventory", http.StatusBadRequest)
		default:
			t.Errorf("unexpected search request: %s", r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	return cloudPulseSearchOptions(t, server), calls
}

type pulsePaginationOutput struct {
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
		ID             string `json:"id"`
		UpstreamStatus int    `json:"upstream_status"`
	} `json:"error"`
}

func TestPulseSearchStopsBeforeUnusedDefinitionSource(t *testing.T) {
	for _, scenario := range []struct {
		selector  string
		limit     int
		wantError bool
		wantMore  bool
	}{
		{"pulse", 100, false, true},
		{"pulse", 101, false, true},
		{"pulse", 130, false, true},
		{"pulse", 131, true, false},
		{"metric", 101, false, true},
		{"metric", 130, false, false},
	} {
		t.Run(fmt.Sprintf("%s_limit%d", scenario.selector, scenario.limit), func(t *testing.T) {
			options, calls := pulsePaginationOptions(t, 130, 0, http.StatusTooManyRequests)
			var output strings.Builder
			code := app.Run(t.Context(), []string{"search", "test", "--type", scenario.selector, "--limit", strconv.Itoa(scenario.limit), "--environment", "test", "--json"}, &output, options)
			var result pulsePaginationOutput
			if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
				t.Fatal(err)
			}
			t.Logf("selector=%s limit=%d code=%d returned=%d native_calls=%d definition_calls=%d metric_list_calls=%d error=%s upstream_status=%d", scenario.selector, scenario.limit, code, result.Page.Returned, calls.native.Load(), calls.definitions.Load(), calls.metricLists.Load(), result.Error.ID, result.Error.UpstreamStatus)
			wantNative := int32(2)
			if scenario.limit == 100 {
				wantNative = 1
			}
			if calls.metricLists.Load() != 0 || calls.native.Load() != wantNative {
				t.Fatalf("unexpected source traversal: native=%d metric-lists=%d", calls.native.Load(), calls.metricLists.Load())
			}
			if scenario.wantError {
				if code == 0 || calls.definitions.Load() != 1 || result.Error.UpstreamStatus != 429 || len(result.Items) != 0 {
					t.Fatalf("needed definition source failure was not preserved: %s", output.String())
				}
				return
			}
			if code != 0 || calls.definitions.Load() != 0 || result.Page.Returned != scenario.limit || len(result.Items) != scenario.limit {
				t.Fatalf("requested %d native metric rows were available, but search consulted an unused definition source: %s", scenario.limit, output.String())
			}
			if result.Page.MoreAvailable != scenario.wantMore {
				t.Fatalf("more_available=%t want=%t", result.Page.MoreAvailable, scenario.wantMore)
			}
			for index, item := range result.Items {
				if item.Type != "metric" || item.LUID != fmt.Sprintf("metric-%04d", index) {
					t.Fatalf("native rank changed at %d: %+v", index, item)
				}
			}
		})
	}
}

func TestPulseSearchReadsDefinitionPagesOncePerCommand(t *testing.T) {
	for _, scenario := range []struct {
		selector           string
		nativeCount, limit int
	}{
		{"definition", 0, 1850},
		{"pulse", 150, 2000},
	} {
		t.Run(scenario.selector, func(t *testing.T) {
			options, calls := pulsePaginationOptions(t, scenario.nativeCount, 1850, 0)
			var output strings.Builder
			code := app.Run(t.Context(), []string{"search", "test", "--type", scenario.selector, "--limit", strconv.Itoa(scenario.limit), "--environment", "test", "--json"}, &output, options)
			var result pulsePaginationOutput
			if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
				t.Fatal(err)
			}
			if code != 0 || result.Page.Returned != scenario.limit || len(result.Items) != scenario.limit || result.Page.MoreAvailable {
				t.Fatalf("search paging failed: code=%d output=%s", code, output.String())
			}
			for index, item := range result.Items {
				wantType, wantID := "definition", fmt.Sprintf("definition-%04d", index-scenario.nativeCount)
				if index < scenario.nativeCount {
					wantType, wantID = "metric", fmt.Sprintf("metric-%04d", index)
				}
				if item.Type != wantType || item.LUID != wantID {
					t.Fatalf("grouped rank/identity changed at %d: got=%+v want=%s/%s", index, item, wantType, wantID)
				}
			}
			if calls.metricLists.Load() != 0 {
				t.Fatalf("term search used a per-definition metric inventory: %d", calls.metricLists.Load())
			}
			calls.mu.Lock()
			tokens := slices.Clone(calls.definitionTokens)
			calls.mu.Unlock()
			unique := make(map[string]bool)
			for _, token := range tokens {
				unique[token] = true
			}
			t.Logf("selector=%s returned=%d native_calls=%d definition_requests=%d distinct_definition_pages=%d", scenario.selector, result.Page.Returned, calls.native.Load(), calls.definitions.Load(), len(unique))
			if len(tokens) != 19 || len(unique) != 19 || calls.definitions.Load() != 19 {
				t.Fatalf("one invocation refetched matching definition pages: total_requests=%d distinct_pages=%d", len(tokens), len(unique))
			}
		})
	}
}

func TestPulseDefinitionSearchRefreshesPagesBetweenCommands(t *testing.T) {
	options, calls := pulsePaginationOptions(t, 0, 100, 0)
	args := []string{"search", "test", "--type", "definition", "--limit", "1", "--environment", "test", "--json"}
	for generation := range 2 {
		calls.generation.Store(int32(generation))
		var output strings.Builder
		if code := app.Run(t.Context(), args, &output, options); code != 0 {
			t.Fatalf("command %d exit=%d output=%s", generation+1, code, output.String())
		}
		var result pulsePaginationOutput
		if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
			t.Fatal(err)
		}
		wantName := fmt.Sprintf("test Definition 0000 version %d", generation)
		if len(result.Items) != 1 || result.Items[0].LUID != "definition-0000" || result.Items[0].Name != wantName || !result.Page.MoreAvailable {
			t.Fatalf("command %d reused stale results: %+v", generation+1, result)
		}
	}
	if calls.definitions.Load() != 2 || calls.native.Load() != 0 || calls.metricLists.Load() != 0 {
		t.Fatalf("each command must read its own current page: definitions=%d native=%d metric-lists=%d", calls.definitions.Load(), calls.native.Load(), calls.metricLists.Load())
	}
}

func TestPulseDefinitionSearchReadsAgainAfterFailedCommand(t *testing.T) {
	options, calls := pulsePaginationOptions(t, 0, 1, http.StatusTooManyRequests)
	args := []string{"search", "test", "--type", "definition", "--limit", "1", "--environment", "test", "--json"}
	var firstOutput strings.Builder
	if code := app.Run(t.Context(), args, &firstOutput, options); code == 0 {
		t.Fatal("throttled command succeeded")
	}
	var first pulsePaginationOutput
	if err := json.Unmarshal([]byte(firstOutput.String()), &first); err != nil {
		t.Fatal(err)
	}
	if first.Error.UpstreamStatus != 429 || len(first.Items) != 0 || calls.definitions.Load() != 1 {
		t.Fatalf("unexpected first command failure: %+v calls=%d", first, calls.definitions.Load())
	}
	calls.definitionStatus.Store(0)
	var secondOutput strings.Builder
	if code := app.Run(t.Context(), args, &secondOutput, options); code != 0 {
		t.Fatalf("second command retained prior failure: code=%d output=%s", code, secondOutput.String())
	}
	var second pulsePaginationOutput
	if err := json.Unmarshal([]byte(secondOutput.String()), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].LUID != "definition-0000" || second.Page.Returned != 1 || second.Page.MoreAvailable || calls.definitions.Load() != 2 {
		t.Fatalf("second command must fetch successful current result: %+v calls=%d", second, calls.definitions.Load())
	}
}
