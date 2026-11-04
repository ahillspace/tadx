package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestCloudTermSearchUsesServerSidePulseDiscovery(t *testing.T) {
	for _, scenario := range []struct {
		selector     string
		nativeFilter string
		wantIDs      []string
		definitions  int32
	}{
		{selector: "metric", nativeFilter: "type:eq:insightmetric", wantIDs: []string{"metric-1"}},
		{selector: "pulse", nativeFilter: "type:eq:insightmetric", wantIDs: []string{"metric-1", "definition-1"}, definitions: 1},
		{selector: "", nativeFilter: "type:in:[datasource,flow,insightmetric,project,workbook]", wantIDs: []string{"workbook-1", "metric-1", "definition-1"}, definitions: 1},
		{selector: "content", nativeFilter: "type:in:[datasource,flow,project,workbook]", wantIDs: []string{"workbook-1"}},
		{selector: "definition", wantIDs: []string{"definition-1"}, definitions: 1},
	} {
		t.Run("selector="+scenario.selector, func(t *testing.T) {
			fixture := &pulseTermSearchFixture{
				term: "test", nativeFilter: scenario.nativeFilter, definitionFilter: "name:has:test",
				includeWorkbook: scenario.selector == "" || scenario.selector == "content",
				includeMetric:   scenario.selector == "" || scenario.selector == "metric" || scenario.selector == "pulse",
			}
			options := fixture.options(t)
			var output strings.Builder
			args := []string{"search", "test", "--environment", "test", "--json"}
			if scenario.selector != "" {
				args = append(args, "--type", scenario.selector)
			}
			if code := app.Run(t.Context(), args, &output, options); code != 0 {
				t.Fatalf("search exit=%d output=%s", code, output.String())
			}
			var result struct {
				Items []struct {
					LUID string `json:"luid"`
					Type string `json:"type"`
				} `json:"items"`
				Page struct {
					Returned      int  `json:"returned"`
					MoreAvailable bool `json:"more_available"`
				} `json:"page"`
			}
			if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(result.Items))
			for i, item := range result.Items {
				ids[i] = item.LUID
				if item.LUID == "metric-1" && item.Type != "metric" {
					t.Fatalf("native Pulse result has type %q", item.Type)
				}
			}
			if !slices.Equal(ids, scenario.wantIDs) || result.Page.Returned != len(scenario.wantIDs) || result.Page.MoreAvailable {
				t.Fatalf("identities=%v page=%+v want=%v", ids, result.Page, scenario.wantIDs)
			}
			wantNative := int32(1)
			if scenario.nativeFilter == "" {
				wantNative = 0
			}
			if fixture.nativeCalls.Load() != wantNative || fixture.definitionCalls.Load() != scenario.definitions || fixture.metricCalls.Load() != 0 {
				t.Fatalf("requests: native=%d definitions=%d metric-lists=%d", fixture.nativeCalls.Load(), fixture.definitionCalls.Load(), fixture.metricCalls.Load())
			}
		})
	}
}

func TestCloudTermSearchEmptyMatchesDoNotScanPulseInventory(t *testing.T) {
	for _, selector := range []string{"metric", "pulse", ""} {
		t.Run("selector="+selector, func(t *testing.T) {
			nativeFilter := "type:eq:insightmetric"
			if selector == "" {
				nativeFilter = "type:in:[datasource,flow,insightmetric,project,workbook]"
			}
			fixture := &pulseTermSearchFixture{term: "missing", nativeFilter: nativeFilter, definitionFilter: "name:has:missing", empty: true}
			options := fixture.options(t)
			var output strings.Builder
			args := []string{"search", "missing", "--environment", "test", "--json"}
			if selector != "" {
				args = append(args, "--type", selector)
			}
			if code := app.Run(t.Context(), args, &output, options); code != 0 {
				t.Fatalf("search exit=%d output=%s", code, output.String())
			}
			var result struct {
				Items []json.RawMessage `json:"items"`
				Page  struct {
					Returned      int  `json:"returned"`
					MoreAvailable bool `json:"more_available"`
				} `json:"page"`
			}
			if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 0 || result.Page.Returned != 0 || result.Page.MoreAvailable {
				t.Fatalf("empty search returned %+v", result)
			}
			wantDefinitions := int32(1)
			if selector == "metric" {
				wantDefinitions = 0
			}
			if fixture.nativeCalls.Load() != 1 || fixture.definitionCalls.Load() != wantDefinitions || fixture.metricCalls.Load() != 0 {
				t.Fatalf("empty search scanned inventory: native=%d definitions=%d metric-lists=%d", fixture.nativeCalls.Load(), fixture.definitionCalls.Load(), fixture.metricCalls.Load())
			}
		})
	}
}

func TestCloudDefinitionSearchEncodesReservedAndUnicodeTerm(t *testing.T) {
	const term = "Ops,EMEA & Café / % + : [x]"
	fixture := &pulseTermSearchFixture{
		term: term, definitionFilter: "name:has:Ops%2CEMEA%20%26%20Caf%C3%A9%20%2F%20%25%20%2B%20%3A%20%5Bx%5D",
	}
	var output strings.Builder
	if code := app.Run(t.Context(), []string{"search", term, "--type", "definition", "--environment", "test", "--json"}, &output, fixture.options(t)); code != 0 {
		t.Fatalf("encoded search exit=%d output=%s", code, output.String())
	}
	if !strings.Contains(output.String(), `"luid":"definition-1"`) || fixture.definitionCalls.Load() != 1 {
		t.Fatalf("definition requests=%d output=%s", fixture.definitionCalls.Load(), output.String())
	}
}

func TestCloudMetricSearchPreservesUpstreamThrottleAdvice(t *testing.T) {
	fixture := &pulseTermSearchFixture{term: "test", nativeFilter: "type:eq:insightmetric", throttle: true}
	var output strings.Builder
	if code := app.Run(t.Context(), []string{"search", "test", "--type", "metric", "--environment", "test", "--json"}, &output, fixture.options(t)); code == 0 {
		t.Fatal("throttled search succeeded")
	}
	var result struct {
		Error struct {
			Retryable        bool   `json:"retryable"`
			UpstreamStatus   int    `json:"upstream_status"`
			CorrectiveAction string `json:"corrective_action"`
			TableauRequestID string `json:"tableau_request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Error.Retryable || result.Error.UpstreamStatus != 429 || result.Error.TableauRequestID != "pulse-search-throttled" || !strings.Contains(result.Error.CorrectiveAction, "Wait") || strings.Contains(result.Error.CorrectiveAction, "filters") {
		t.Fatalf("upstream throttle advice lost: %+v", result.Error)
	}
	if fixture.nativeCalls.Load() != 1 || fixture.definitionCalls.Load() != 0 || fixture.metricCalls.Load() != 0 {
		t.Fatalf("throttled search retried or scanned inventories: native=%d definitions=%d metric-lists=%d", fixture.nativeCalls.Load(), fixture.definitionCalls.Load(), fixture.metricCalls.Load())
	}
}

func TestCloudMetricSearchWithoutTermRetainsInventoryTraversal(t *testing.T) {
	var definitionCalls, metricCalls atomic.Int32
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/-/pulse/definitions":
			definitionCalls.Add(1)
			if r.URL.Query().Get("filter") != "" {
				t.Error("no-term inventory applied a name filter")
			}
			_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}],"next_page_token":""}`)
		case "/api/-/pulse/definitions/definition-1/metrics":
			metricCalls.Add(1)
			_, _ = io.WriteString(w, `{"metrics":[{"id":"metric-1","definition_id":"definition-1","is_default":true}],"next_page_token":""}`)
		default:
			t.Errorf("unexpected no-term inventory request: %s", r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	var output strings.Builder
	if code := app.Run(t.Context(), []string{"search", "--type", "metric", "--environment", "test", "--json"}, &output, cloudPulseSearchOptions(t, server)); code != 0 {
		t.Fatalf("inventory exit=%d output=%s", code, output.String())
	}
	if !strings.Contains(output.String(), `"luid":"metric-1"`) || definitionCalls.Load() != 1 || metricCalls.Load() != 1 {
		t.Fatalf("inventory requests definitions=%d metrics=%d output=%s", definitionCalls.Load(), metricCalls.Load(), output.String())
	}
}

type pulseTermSearchFixture struct {
	term, nativeFilter, definitionFilter string
	includeWorkbook, includeMetric       bool
	empty, throttle                      bool
	nativeCalls, definitionCalls         atomic.Int32
	metricCalls                          atomic.Int32
}

func (fixture *pulseTermSearchFixture) options(t *testing.T) app.Options {
	t.Helper()
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/api/-/search":
			fixture.nativeCalls.Add(1)
			query := r.URL.Query()
			if fixture.nativeFilter == "" || query.Get("filter") != fixture.nativeFilter || query.Get("terms") != fixture.term {
				t.Errorf("unexpected native search query: %s", r.URL.RawQuery)
				http.Error(w, "wrong native search scope", http.StatusBadRequest)
				return
			}
			if fixture.throttle {
				w.Header().Set("Retry-After", "28")
				w.Header().Set("X-Tableau-Request-Id", "pulse-search-throttled")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"httpErrorCode":429,"message":"Too Many Requests"}`)
				return
			}
			items := []any{}
			if fixture.includeWorkbook {
				items = append(items, map[string]any{"uri": "/workbooks/workbook-1", "content": map[string]any{"type": "workbook", "luid": "workbook-1", "title": fixture.term + " Workbook"}})
			}
			if fixture.includeMetric {
				items = append(items, map[string]any{"uri": "/metrics/search-index-id", "content": map[string]any{"type": "insightmetric", "id": "search-index-id", "luid": "noncanonical-luid", "metricId": "metric-1", "title": fixture.term + " Metric"}})
			}
			limit, err := strconv.Atoi(query.Get("limit"))
			if err != nil || limit < 1 || limit > 100 || query.Get("page") != "0" {
				t.Errorf("unbounded native search query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "limit": limit, "pageIndex": 0, "startIndex": 0, "total": len(items)})
		case r.URL.Path == "/api/-/pulse/definitions":
			fixture.definitionCalls.Add(1)
			query := r.URL.Query()
			// A large unrelated site inventory is unavailable to term search.
			// Only a correctly scoped server query can return the matching row.
			if query.Get("filter") != fixture.definitionFilter || query.Get("page_size") != "100" || query.Get("page_token") != "" {
				t.Errorf("term search attempted unfiltered Pulse inventory: %s", r.URL.RawQuery)
				http.Error(w, "unfiltered inventory is forbidden", http.StatusBadRequest)
				return
			}
			definitions := []any{}
			if !fixture.empty {
				definitions = append(definitions, map[string]any{"metadata": map[string]any{"id": "definition-1", "name": fixture.term + " Definition"}, "specification": map[string]any{"datasource": map[string]any{"id": "datasource-1"}}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"definitions": definitions, "next_page_token": ""})
		case strings.HasPrefix(r.URL.Path, "/api/-/pulse/definitions/") && strings.HasSuffix(r.URL.Path, "/metrics"):
			fixture.metricCalls.Add(1)
			t.Error("term search enumerated metrics under a definition")
			http.Error(w, "per-definition metric scans are forbidden", http.StatusTooManyRequests)
		case r.URL.Path == "/api/3.29/sites/site-1/groups" || r.URL.Path == "/api/3.29/sites/site-1/users":
			kind := "groups"
			if strings.HasSuffix(r.URL.Path, "/users") {
				kind = "users"
			}
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="0"/><%s/></tsResponse>`, r.URL.Query().Get("pageSize"), kind)
		default:
			t.Errorf("unexpected term search request: %s", r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	return cloudPulseSearchOptions(t, server)
}

func cloudPulseSearchOptions(t *testing.T, server *httptest.Server) app.Options {
	t.Helper()
	options := diagnosticOptions(t, server)
	if _, err := config.Update(options.ConfigPath, false, func(cfg config.Config) (config.Config, error) {
		environment := cfg.Environments["test"]
		environment.URL = "https://prod-test.online.tableau.com"
		cfg.Environments["test"] = environment
		return cfg, nil
	}); err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := *server.Client()
	client.Transport = targetFixtureTransport{endpoint: endpoint, base: client.Transport}
	options.HTTPClient = &client
	return options
}
