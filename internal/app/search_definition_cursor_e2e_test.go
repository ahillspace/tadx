package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/config"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type definitionCursorTransport struct {
	endpoint *url.URL
	base     http.RoundTripper
}

func (transport definitionCursorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	local := request.Clone(request.Context())
	local.URL.Scheme, local.URL.Host = transport.endpoint.Scheme, transport.endpoint.Host
	return transport.base.RoundTrip(local)
}

type definitionCursorCalls struct{ definitions, other atomic.Int32 }

func definitionCursorFixture(t *testing.T, filter string) (Options, *tableaupulse.Client, *definitionCursorCalls) {
	t.Helper()
	calls := &definitionCursorCalls{}
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/3.29/auth/signin" {
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-luid"},"user":{"id":"user-luid"}}}`)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/-/pulse/definitions" {
			calls.other.Add(1)
			t.Errorf("unexpected cursor search request: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		calls.definitions.Add(1)
		query := r.URL.Query()
		if query.Get("filter") != filter || query.Get("page_size") != "100" || (query.Get("page_token") != "" && query.Get("page_token") != "100") {
			t.Errorf("unexpected definition query: %s", r.URL.RawQuery)
		}
		start := 0
		if query.Get("page_token") == "100" {
			start = 100
		}
		definitions := make([]any, 100)
		for index := range definitions {
			definitions[index] = map[string]any{"metadata": map[string]string{"id": fmt.Sprintf("definition-%03d", start+index), "name": fmt.Sprintf("test %03d", start+index)}, "specification": map[string]any{"datasource": map[string]string{"id": "datasource-1"}}}
		}
		next := ""
		if start == 0 {
			next = "100"
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"definitions": definitions, "next_page_token": next}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	options := overviewOptions(t, t.TempDir())
	if err := config.Save(options.ConfigPath, config.Config{Version: config.CurrentVersion, DefaultEnvironment: "test", Environments: map[string]config.Environment{
		"test": {URL: "https://fixture.online.tableau.com", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "DEFINITION_CURSOR_PAT_NAME", PATSecretEnv: "DEFINITION_CURSOR_PAT_SECRET"}},
	}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEFINITION_CURSOR_PAT_NAME", "fixture-pat")
	t.Setenv("DEFINITION_CURSOR_PAT_SECRET", "fixture-pat-secret")
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := *server.Client()
	client.Transport = definitionCursorTransport{endpoint: endpoint, base: client.Transport}
	options.HTTPClient = &client
	pulse, err := tableaupulse.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), cacheTestSession{}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return options, pulse, calls
}

type legacyDefinitionInventory struct{ t *testing.T }

func (inventory legacyDefinitionInventory) List(_ context.Context, kind, cursor string, limit int) (resourcesearch.Page, error) {
	if kind != "definition" || cursor != "" || limit != 100 {
		inventory.t.Fatalf("legacy setup list kind=%q cursor=%q limit=%d", kind, cursor, limit)
	}
	page := resourcesearch.Page{NextCursor: "100"}
	for index := range 80 {
		page.Items = append(page.Items, resourcesearch.Item{LUID: fmt.Sprintf("other-%03d", index), Type: "definition", Name: fmt.Sprintf("aaa other %03d", index)})
	}
	for index := range 20 {
		page.Items = append(page.Items, resourcesearch.Item{LUID: fmt.Sprintf("definition-%03d", index), Type: "definition", Name: fmt.Sprintf("test %03d", index)})
	}
	return page, nil
}

func checkDefinitionCursorItems(t *testing.T, items []searchaction.Item, start, count int) {
	t.Helper()
	if len(items) != count {
		t.Fatalf("definition rows = %d, want %d", len(items), count)
	}
	for index, item := range items {
		if item.Type != "definition" || item.LUID != fmt.Sprintf("definition-%03d", start+index) || item.Name != fmt.Sprintf("test %03d", start+index) {
			t.Fatalf("definition identity/order changed at %d: %+v", index, item)
		}
	}
}

func runDefinitionCursor(t *testing.T, options Options, terms, cursor string, cached bool) searchaction.Output {
	t.Helper()
	args := []string{"search", "--type", "definition", "--limit", "20", "--cursor", cursor, "--json"}
	if terms != "" {
		args = append(args, terms)
	}
	if cached {
		args = append(args, "--cache")
	}
	var output strings.Builder
	if code := Run(t.Context(), args, &output, options); code != 0 {
		t.Fatalf("continuation exit=%d output=%s", code, output.String())
	}
	var result searchaction.Output
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDefinitionSearchRejectsLegacyInventoryCursorBeforeFilteredRequest(t *testing.T) {
	legacy, err := searchaction.Execute(t.Context(), globalSearchSource{dedicated: resourcesearch.NewAdapter(legacyDefinitionInventory{t})}, searchaction.Input{Terms: "test", Type: "definition", Environment: "test", SiteResolved: true, Limit: 20}, []string{"definition"})
	if err != nil || legacy.Page.NextCursor == "" || !legacy.Page.MoreAvailable {
		t.Fatalf("legacy inventory cursor setup: page=%+v error=%v", legacy.Page, err)
	}
	checkDefinitionCursorItems(t, legacy.Items, 0, 20)
	options, _, calls := definitionCursorFixture(t, "name:has:test")
	var output strings.Builder
	code := Run(t.Context(), []string{"search", "test", "--type", "definition", "--limit", "20", "--environment", "test", "--cursor", legacy.Page.NextCursor, "--json"}, &output, options)
	var result struct {
		Items []searchaction.Item `json:"items"`
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	if code != 2 || result.Error.Kind != "usage" || len(result.Items) != 0 || calls.definitions.Load() != 0 || calls.other.Load() != 0 {
		t.Fatalf("legacy inventory continuation reached the filtered source: code=%d definition-calls=%d other-calls=%d output=%s", code, calls.definitions.Load(), calls.other.Load(), output.String())
	}
}

func TestDefinitionSearchAcceptsCurrentFilteredCursorThroughCLI(t *testing.T) {
	options, _, calls := definitionCursorFixture(t, "name:has:test")
	runtime, err := newRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	first, err := newSearchCommands(runtime).Execute(t.Context(), searchaction.Input{Terms: "test", Type: "definition", Environment: "test", Limit: 20})
	if closeErr := runtime.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || first.Page.NextCursor == "" || !first.Page.MoreAvailable {
		t.Fatalf("current filtered cursor setup: page=%+v error=%v", first.Page, err)
	}
	checkDefinitionCursorItems(t, first.Items, 0, 20)
	next := runDefinitionCursor(t, options, "test", first.Page.NextCursor, false)
	checkDefinitionCursorItems(t, next.Items, 20, 20)
	if next.Page.Returned != 20 || !next.Page.MoreAvailable || calls.definitions.Load() != 2 || calls.other.Load() != 0 {
		t.Fatalf("current filtered continuation page=%+v definition-calls=%d other-calls=%d", next.Page, calls.definitions.Load(), calls.other.Load())
	}
}

func TestDefinitionSearchPreservesLegacyNoTermCursor(t *testing.T) {
	options, client, calls := definitionCursorFixture(t, "")
	legacy, err := searchaction.Execute(t.Context(), globalSearchSource{dedicated: resourcesearch.NewAdapter(&liveSearchLister{environment: "test", pulse: client})}, searchaction.Input{Type: "definition", Environment: "test", SiteResolved: true, Limit: 20}, []string{"definition"})
	if err != nil || legacy.Page.NextCursor == "" {
		t.Fatalf("legacy no-term cursor setup: page=%+v error=%v", legacy.Page, err)
	}
	checkDefinitionCursorItems(t, legacy.Items, 0, 20)
	next := runDefinitionCursor(t, options, "", legacy.Page.NextCursor, false)
	checkDefinitionCursorItems(t, next.Items, 20, 20)
	if next.Page.Returned != 20 || !next.Page.MoreAvailable || calls.definitions.Load() != 2 || calls.other.Load() != 0 {
		t.Fatalf("legacy no-term continuation page=%+v definition-calls=%d other-calls=%d", next.Page, calls.definitions.Load(), calls.other.Load())
	}
}

func TestDefinitionSearchPreservesLegacyCachedTermCursor(t *testing.T) {
	options, store, replacement := cachedSearchSessionFixture(t, 40)
	options.HTTPClient = &http.Client{Transport: overviewForbiddenTransport{t}}
	options.PATStore = overviewForbiddenStore{t: t}
	for index := range replacement.Entries {
		replacement.Entries[index].Environment = "fixture"
		replacement.Entries[index].Site = "fixture-site"
		replacement.Entries[index].Kind = "definition"
		replacement.Entries[index].LUID = fmt.Sprintf("definition-%03d", index)
		replacement.Entries[index].Name = fmt.Sprintf("test %03d", index)
		replacement.Entries[index].Coverage = "summary"
		replacement.Entries[index].ObservedAt = options.Now()
	}
	if err := store.UpsertResources(t.Context(), replacement.Entries); err != nil {
		t.Fatal(err)
	}
	reader, err := store.OpenResourceRead(t.Context(), "fixture", "fixture-site")
	if err != nil {
		t.Fatal(err)
	}
	legacy, searchErr := searchaction.Execute(t.Context(), cacheGlobalSearchSource{reader: reader}, searchaction.Input{Terms: "test", Type: "definition", Environment: "fixture", Site: "fixture-site", SiteResolved: true, Cache: true, Limit: 20}, []string{"definition"})
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if searchErr != nil || legacy.Page.NextCursor == "" {
		t.Fatalf("legacy cache cursor setup: page=%+v error=%v", legacy.Page, searchErr)
	}
	checkDefinitionCursorItems(t, legacy.Items, 0, 20)
	next := runDefinitionCursor(t, options, "test", legacy.Page.NextCursor, true)
	checkDefinitionCursorItems(t, next.Items, 20, 20)
	if next.Page.Returned != 20 || next.Page.MoreAvailable || next.Source != "cache" {
		t.Fatalf("legacy cached continuation page=%+v source=%q", next.Page, next.Source)
	}
}
