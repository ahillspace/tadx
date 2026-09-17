package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	searchaction "github.com/ahillspace/tadx/actions/search"
)

func TestSearchIdentityCLIPreservesMixedRankAndPublishedParents(t *testing.T) {
	hits := []json.RawMessage{
		searchIdentityWorkbook(t, "workbook-first"),
		searchIdentityDatasource(t, "connection-a-1", "datasource-a", "regional-a", true),
		searchIdentityDatasource(t, "embedded-connection", "embedded-datasource", "embedded-only", false),
		searchIdentityDatasource(t, "connection-a-2", "datasource-a", "regional-a", true),
		searchIdentityDatasource(t, "connection-b", "datasource-b", "regional-b", true),
		searchIdentityWorkbook(t, "workbook-last"),
	}
	server := newSearchIdentityServer(t, hits)
	out := runSearchIdentityCLI(t, server, 20)
	searchIdentityWant(t, out.Items, []string{"workbook-first", "datasource-a", "datasource-b", "workbook-last"})
	if out.Page.Returned != 4 || out.Page.MoreAvailable {
		t.Fatalf("page = %+v", out.Page)
	}
	if out.Items[1].Name != out.Items[2].Name || out.Items[1].Type != "datasource" || out.Items[2].Type != "datasource" {
		t.Fatalf("same-name published datasources were changed: %+v", out.Items)
	}
}

func TestSearchIdentityContinuationSkipsPriorParentAndFillsUniqueLimit(t *testing.T) {
	hits := []json.RawMessage{
		searchIdentityWorkbook(t, "workbook-first"),
		searchIdentityDatasource(t, "connection-a-1", "datasource-a", "regional-a", true),
		searchIdentityDatasource(t, "connection-a-2", "datasource-a", "regional-a", true),
		searchIdentityWorkbook(t, "workbook-second"),
		searchIdentityDatasource(t, "connection-b", "datasource-b", "regional-b", true),
		searchIdentityWorkbook(t, "workbook-last"),
	}
	server := newSearchIdentityServer(t, hits)
	commands := newSearchCommands(inventoryListRuntime(t, server))
	input := searchaction.Input{Environment: "production", Terms: "Regional", Type: "content", Limit: 2}
	first, err := commands.Execute(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	searchIdentityWant(t, first.Items, []string{"workbook-first", "datasource-a"})
	if first.Page.NextCursor == "" {
		t.Fatal("first page lost native continuation")
	}
	input.Cursor = first.Page.NextCursor
	second, err := commands.Execute(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	searchIdentityWant(t, second.Items, []string{"workbook-second", "datasource-b"})
	if second.Page.Returned != 2 || second.Page.NextCursor == "" {
		t.Fatalf("second page did not fill its unique-row limit and preserve continuation: %+v", second.Page)
	}
	input.Cursor = second.Page.NextCursor
	third, err := commands.Execute(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	searchIdentityWant(t, third.Items, []string{"workbook-last"})
	if third.Page.MoreAvailable || third.Page.NextCursor != "" {
		t.Fatalf("final page = %+v", third.Page)
	}
}

func TestSearchIdentityCLIExpandedLimitDeduplicatesAcrossNativePages(t *testing.T) {
	hits := make([]json.RawMessage, 0, 103)
	want := make([]string, 0, 102)
	for index := range 99 {
		luid := fmt.Sprintf("workbook-%03d", index)
		hits = append(hits, searchIdentityWorkbook(t, luid))
		want = append(want, luid)
	}
	hits = append(hits,
		searchIdentityDatasource(t, "connection-a-1", "datasource-a", "regional-a", true),
		searchIdentityDatasource(t, "connection-a-2", "datasource-a", "regional-a", true),
		searchIdentityWorkbook(t, "workbook-after-duplicate"),
		searchIdentityWorkbook(t, "workbook-tail"),
	)
	want = append(want, "datasource-a", "workbook-after-duplicate", "workbook-tail")
	out := runSearchIdentityCLI(t, newSearchIdentityServer(t, hits), 102)
	searchIdentityWant(t, out.Items, want)
	if out.Page.Returned != 102 || out.Page.Limit != 102 || out.Page.MoreAvailable {
		t.Fatalf("expanded page = %+v", out.Page)
	}
}

type searchIdentityOutput struct {
	Items []searchaction.Item `json:"items"`
	Page  searchaction.Page   `json:"page"`
}

func runSearchIdentityCLI(t *testing.T, server *httptest.Server, limit int) searchIdentityOutput {
	t.Helper()
	runtime := inventoryListRuntime(t, server)
	var stdout bytes.Buffer
	code := Run(t.Context(), []string{"search", "Regional", "--type", "content", "--environment", "production", "--limit", strconv.Itoa(limit), "--json"}, &stdout, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client()})
	if code != 0 {
		t.Fatalf("search exit=%d output=%s", code, stdout.String())
	}
	var out searchIdentityOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode search output: %v; output=%s", err, stdout.String())
	}
	return out
}

func searchIdentityWant(t *testing.T, items []searchaction.Item, want []string) {
	t.Helper()
	got := make([]string, len(items))
	for index, item := range items {
		got[index] = item.LUID
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ranked identities = %v; want %v", got, want)
	}
}

func searchIdentityWorkbook(t *testing.T, luid string) json.RawMessage {
	t.Helper()
	return searchIdentityJSON(t, map[string]any{"uri": "/workbooks/" + luid, "content": map[string]any{
		"type": "workbook", "luid": luid, "title": "Regional Workbook", "projectLuid": "project-workbooks", "projectName": "Workbooks",
	}})
}

func searchIdentityDatasource(t *testing.T, searchLUID, parentLUID, repositoryURL string, published bool) json.RawMessage {
	t.Helper()
	parentType := "Datasource"
	containerLUID := parentLUID
	if !published {
		parentType = "Workbook"
		containerLUID = "embedded-parent-workbook"
	}
	return searchIdentityJSON(t, map[string]any{"uri": "/datasources/" + searchLUID, "content": map[string]any{
		"type": "unifieddatasource", "luid": searchLUID, "title": "Regional Superstore", "repositoryUrl": repositoryURL,
		"datasourceLuid": parentLUID, "datasourceIsPublished": published, "parentType": parentType, "parentLuid": containerLUID,
		"projectLuid": "project-" + parentLUID, "projectName": "Project " + parentLUID,
	}})
}

func searchIdentityJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newSearchIdentityServer(t *testing.T, hits []json.RawMessage) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/-/search":
			query := request.URL.Query()
			if query.Get("filter") != "type:in:[datasource,flow,project,workbook]" || query.Get("terms") != "Regional" {
				t.Errorf("mixed native relevance query changed: %s", request.URL.RawQuery)
			}
			limit, limitErr := strconv.Atoi(query.Get("limit"))
			page, pageErr := strconv.Atoi(query.Get("page"))
			if limitErr != nil || pageErr != nil || limit < 1 || limit > 100 || page < 0 || page*limit >= len(hits) {
				t.Errorf("invalid native page request: %s", request.URL.RawQuery)
				http.Error(writer, "invalid page", http.StatusBadRequest)
				return
			}
			start := page * limit
			end := min(start+limit, len(hits))
			next := ""
			if end < len(hits) {
				next = fmt.Sprintf("/api/-/search?page=%d", page+1)
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{"hits": map[string]any{"items": hits[start:end], "limit": limit, "pageIndex": page, "startIndex": start, "total": len(hits), "next": next}})
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			filter := request.URL.Query().Get("filter")
			values, ok := strings.CutPrefix(filter, "contentUrl:eq:")
			if !ok {
				values, ok = strings.CutPrefix(filter, "contentUrl:in:[")
				if ok {
					values, ok = strings.CutSuffix(values, "]")
				}
			}
			if !ok {
				t.Errorf("unexpected datasource identity filter: %s", filter)
				http.Error(writer, "unexpected identity filter", http.StatusBadRequest)
				return
			}
			var rows []string
			for value := range strings.SplitSeq(values, ",") {
				luid := map[string]string{"regional-a": "datasource-a", "regional-b": "datasource-b"}[value]
				if luid == "" {
					t.Errorf("search tried to resolve an embedded or unknown datasource: %q", value)
					http.Error(writer, "unknown published datasource", http.StatusBadRequest)
					return
				}
				rows = append(rows, fmt.Sprintf(`<datasource id="%s" name="Regional Superstore" contentUrl="%s"><project id="project-%s" name="Project %s"/></datasource>`, luid, value, luid, luid))
			}
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="%d"/><datasources>%s</datasources></tsResponse>`, request.URL.Query().Get("pageSize"), len(rows), strings.Join(rows, ""))
		default:
			t.Errorf("unexpected search request: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
