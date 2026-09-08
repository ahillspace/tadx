package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/errs"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
)

func TestCatalogGlobalSearchIncludesReadThroughResourcesWithoutGeneration(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(t.TempDir(), func() time.Time { return now })
	if err := store.UpsertResources(context.Background(), []catalog.ResourceEntry{{Environment: "dev", Site: "site", Kind: "workbook", LUID: "wb-1", Name: "Sales", Coverage: "summary", ObservedAt: now}}); err != nil {
		t.Fatal(err)
	}
	out, err := searchaction.New(catalogGlobalSearchSource{store: store}).Execute(context.Background(), searchaction.Input{Terms: "sales", Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Catalog: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].LUID != "wb-1" || out.Generation != nil {
		t.Fatalf("output=%+v", out)
	}
}

func TestCatalogGlobalSearchRejectsExplicitUnavailableType(t *testing.T) {
	store := catalog.NewStore(t.TempDir(), time.Now)
	_, err := searchaction.New(catalogGlobalSearchSource{store: store}).Execute(context.Background(), searchaction.Input{Type: "metric", Environment: "dev", Site: "site", SiteResolved: true, Catalog: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%v", err)
	}
}

type appSearchFake struct {
	pages   []resourcesearch.Page
	inputs  []resourcesearch.Input
	budgets []int
}

type completeListPagerCall struct {
	resourceType string
	cursor       string
	limit        int
	input        resourcesearch.Input
}

type completeListPagerFake struct {
	pages []resourcesearch.Page
	calls []completeListPagerCall
}

func (f *completeListPagerFake) searchPage(_ context.Context, resourceType, cursor string, limit int, input resourcesearch.Input) (resourcesearch.Page, error) {
	f.calls = append(f.calls, completeListPagerCall{resourceType: resourceType, cursor: cursor, limit: limit, input: input})
	if len(f.pages) == 0 {
		return resourcesearch.Page{}, errors.New("unexpected complete list page call")
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

func (s *appSearchFake) Search(_ context.Context, input resourcesearch.Input) (resourcesearch.Page, error) {
	s.inputs = append(s.inputs, input)
	if len(s.pages) == 0 {
		return resourcesearch.Page{}, errors.New("unexpected search call")
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func (s *appSearchFake) SearchBounded(_ context.Context, input resourcesearch.Input, budget int) (resourcesearch.Page, error) {
	s.inputs = append(s.inputs, input)
	s.budgets = append(s.budgets, budget)
	if len(s.pages) == 0 {
		return resourcesearch.Page{}, errors.New("unexpected bounded search call")
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func TestLiveGlobalSearchRoutesNonemptyContentTermsToNativeSearch(t *testing.T) {
	for _, test := range []struct {
		selector string
		types    []string
	}{{"content", []string{"datasource", "flow", "project", "workbook"}}, {"workbook", []string{"workbook"}}} {
		native := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
		dedicated := &appSearchFake{}
		source := globalSearchSource{native: native, dedicated: dedicated}
		result, err := source.Search(context.Background(), searchaction.Input{Terms: "sales", Type: test.selector, Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) != 1 || len(native.inputs) != 1 || len(dedicated.inputs) != 0 {
			t.Fatalf("selector=%s result=%+v native=%+v dedicated=%+v", test.selector, result, native.inputs, dedicated.inputs)
		}
		if !reflect.DeepEqual(native.inputs[0].Types, test.types) || native.inputs[0].Terms != "sales" || native.inputs[0].Limit != 20 {
			t.Fatalf("selector=%s native input=%+v", test.selector, native.inputs[0])
		}
	}
}

func TestLiveGlobalSearchRetainsDedicatedRoutesForAdminAndPulse(t *testing.T) {
	for _, selector := range []string{"admin", "user", "group", "pulse", "definition", "metric"} {
		native := &appSearchFake{}
		dedicated := &appSearchFake{pages: []resourcesearch.Page{{}}}
		source := globalSearchSource{native: native, dedicated: dedicated}
		if _, err := source.Search(context.Background(), searchaction.Input{Terms: "sales", Type: selector, Limit: 20}); err != nil {
			t.Fatalf("selector=%s error=%v", selector, err)
		}
		if len(native.inputs) != 0 || len(dedicated.inputs) != 1 {
			t.Fatalf("selector=%s native=%d dedicated=%d", selector, len(native.inputs), len(dedicated.inputs))
		}
	}
}

func TestLiveGlobalSearchRetainsListSemanticsForBlankTypedSearch(t *testing.T) {
	for _, selector := range []string{"workbook", "datasource", "flow", "project", "content", "user", "group", "admin"} {
		native := &appSearchFake{}
		lists := &appSearchFake{pages: []resourcesearch.Page{{}}}
		dedicated := &appSearchFake{}
		source := globalSearchSource{native: native, lists: lists, dedicated: dedicated}
		result, err := source.Search(context.Background(), searchaction.Input{Type: selector, Limit: 20})
		if err != nil || len(result.Items) != 0 || len(native.inputs) != 0 || len(lists.inputs) != 1 || len(dedicated.inputs) != 0 {
			t.Fatalf("selector=%s result=%+v error=%v native=%d lists=%d dedicated=%d", selector, result, err, len(native.inputs), len(lists.inputs), len(dedicated.inputs))
		}
	}
}

func TestLiveGlobalSearchRetainsDedicatedListSemanticsForBlankPulseSearch(t *testing.T) {
	for _, selector := range []string{"definition", "metric", "pulse"} {
		native := &appSearchFake{}
		lists := &appSearchFake{}
		dedicated := &appSearchFake{pages: []resourcesearch.Page{{}}}
		source := globalSearchSource{native: native, lists: lists, dedicated: dedicated}
		if _, err := source.Search(context.Background(), searchaction.Input{Type: selector, Limit: 20}); err != nil {
			t.Fatalf("selector=%s error=%v", selector, err)
		}
		if len(native.inputs) != 0 || len(lists.inputs) != 0 || len(dedicated.inputs) != 1 {
			t.Fatalf("selector=%s native=%d lists=%d dedicated=%d", selector, len(native.inputs), len(lists.inputs), len(dedicated.inputs))
		}
	}
}

func TestCompleteListSearchFamilyContinuationPreservesChildLimitAndPhase(t *testing.T) {
	pager := &completeListPagerFake{pages: []resourcesearch.Page{
		{Items: []resourcesearch.Item{{LUID: "datasource-1", Type: "datasource", Name: "Datasource"}}, Source: "live"},
		{Items: []resourcesearch.Item{{LUID: "flow-1", Type: "flow", Name: "Flow A"}}, NextCursor: "flow-next", Source: "live"},
		{Items: []resourcesearch.Item{{LUID: "flow-2", Type: "flow", Name: "Flow B"}}, Source: "catalog"},
		{Items: []resourcesearch.Item{{LUID: "project-1", Type: "project", Name: "Project"}}, Source: "live"},
	}}
	adapter := &completeLiveSearchAdapter{lister: pager}
	input := resourcesearch.Input{Types: []string{"datasource", "flow", "project", "workbook"}, ProjectPath: "Sales/Ops", Owner: "owner-1", Limit: 2}
	first, err := adapter.Search(context.Background(), input)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" || first.Source != "live" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	input.Cursor = first.NextCursor
	second, err := adapter.Search(context.Background(), input)
	if err != nil || len(second.Items) != 2 || second.Items[0].LUID != "flow-2" || second.Items[1].LUID != "project-1" || second.NextCursor == "" || second.Source != "mixed" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	if len(pager.calls) != 4 || pager.calls[1].limit != 1 || pager.calls[2].limit != 1 || pager.calls[2].cursor != "flow-next" {
		t.Fatalf("calls=%+v", pager.calls)
	}
	for _, call := range pager.calls {
		if call.input.ProjectPath != "Sales/Ops" || call.input.Owner != "owner-1" {
			t.Fatalf("filtered input was not preserved: %+v", call)
		}
	}
}

func TestUntypedLiveSearchReturnsNativeContentBeforeDedicatedResults(t *testing.T) {
	native := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	dedicated := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "user-1", Type: "user", Name: "Sales Analyst"}}, NextCursor: "dedicated-page-2"}, {Items: []resourcesearch.Item{{LUID: "user-2", Type: "user", Name: "Sales Manager"}}}}}
	source := globalSearchSource{native: native, dedicated: dedicated}
	input := searchaction.Input{Terms: "sales", Limit: 2}

	first, err := source.Search(context.Background(), input)
	if err != nil || len(first.Items) != 2 || first.Items[0].Type != "workbook" || first.Items[1].Type != "user" || first.Page.NextCursor == "" || len(dedicated.inputs) != 1 || !reflect.DeepEqual(dedicated.budgets, []int{1}) {
		t.Fatalf("first=%+v error=%v dedicated=%d budgets=%v", first, err, len(dedicated.inputs), dedicated.budgets)
	}
	input.Cursor = first.Page.NextCursor
	second, err := source.Search(context.Background(), input)
	if err != nil || len(second.Items) != 1 || second.Items[0].LUID != "user-2" || second.Page.NextCursor != "" || len(native.inputs) != 1 || len(dedicated.inputs) != 2 {
		t.Fatalf("second=%+v error=%v native=%d dedicated=%d", second, err, len(native.inputs), len(dedicated.inputs))
	}
	wantTypes := []string{"definition", "group", "metric", "user"}
	if !reflect.DeepEqual(dedicated.inputs[0].Types, wantTypes) || dedicated.inputs[0].Cursor != "" {
		t.Fatalf("dedicated input=%+v", dedicated.inputs[0])
	}
}

func TestUntypedLiveSearchExactNativeLimitReturnsDedicatedPhaseCursor(t *testing.T) {
	native := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	dedicated := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "group-1", Type: "group", Name: "Sales"}}}}}
	source := globalSearchSource{native: native, dedicated: dedicated}
	input := searchaction.Input{Terms: "sales", Limit: 1}
	first, err := source.Search(context.Background(), input)
	if err != nil || len(first.Items) != 1 || first.Page.NextCursor == "" || len(dedicated.inputs) != 0 {
		t.Fatalf("first=%+v error=%v dedicated=%d", first, err, len(dedicated.inputs))
	}
	input.Cursor = first.Page.NextCursor
	second, err := source.Search(context.Background(), input)
	if err != nil || len(second.Items) != 1 || second.Items[0].Type != "group" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
}

func TestUntypedLiveSearchWithNoNativeMatchesReturnsDedicatedPageImmediately(t *testing.T) {
	native := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{}}}}
	dedicated := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "user-1", Type: "user", Name: "Sales"}}}}}
	source := globalSearchSource{native: native, dedicated: dedicated}
	result, err := source.Search(context.Background(), searchaction.Input{Terms: "sales", Limit: 20})
	if err != nil || len(result.Items) != 1 || result.Items[0].Type != "user" || len(native.inputs) != 1 || len(dedicated.inputs) != 1 || len(dedicated.budgets) != 0 {
		t.Fatalf("result=%+v error=%v native=%d dedicated=%d budgets=%v", result, err, len(native.inputs), len(dedicated.inputs), dedicated.budgets)
	}
}

func TestUntypedLiveSearchPreservesNativeContinuationBeforePhaseChange(t *testing.T) {
	native := &appSearchFake{pages: []resourcesearch.Page{
		{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales A"}}, NextCursor: "native-page-2"},
		{Items: []resourcesearch.Item{{LUID: "wb-2", Type: "workbook", Name: "Sales B"}}},
	}}
	dedicated := &appSearchFake{}
	source := globalSearchSource{native: native, dedicated: dedicated}
	input := searchaction.Input{Terms: "sales", Limit: 1}

	first, err := source.Search(context.Background(), input)
	if err != nil || first.Page.NextCursor == "" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	input.Cursor = first.Page.NextCursor
	second, err := source.Search(context.Background(), input)
	if err != nil || second.Items[0].LUID != "wb-2" || second.Page.NextCursor == "" || native.inputs[1].Cursor != "native-page-2" || len(dedicated.inputs) != 0 {
		t.Fatalf("second=%+v error=%v native=%+v", second, err, native.inputs)
	}
}

func TestUntypedLiveSearchRejectsChangedCompositeCursor(t *testing.T) {
	native := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	source := globalSearchSource{native: native, dedicated: &appSearchFake{}}
	input := searchaction.Input{Terms: "sales", Limit: 1}
	first, err := source.Search(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Cursor = first.Page.NextCursor
	input.Terms = "changed"
	_, err = source.Search(context.Background(), input)
	var invalid interface{ InvalidSearchCursor() bool }
	if !errors.As(err, &invalid) {
		t.Fatalf("error=%v", err)
	}
}

func TestUntypedLiveSearchContinuationRoundTripsThroughActionCursor(t *testing.T) {
	native := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	dedicated := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "group-1", Type: "group", Name: "Sales Team"}}}}}
	action := searchaction.New(globalSearchSource{native: native, dedicated: dedicated})
	input := searchaction.Input{Terms: "sales", Environment: "dev", Site: "site", SiteResolved: true, Limit: 1}

	first, err := action.Execute(context.Background(), input)
	if err != nil || first.Page.NextCursor == "" || first.Items[0].Type != "workbook" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	input.Cursor = first.Page.NextCursor
	second, err := action.Execute(context.Background(), input)
	if err != nil || second.Page.NextCursor != "" || second.Items[0].Type != "group" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
}

func TestUntypedLiveSearchRejectsOversizedNestedContinuation(t *testing.T) {
	native := &appSearchFake{pages: []resourcesearch.Page{{NextCursor: string(make([]byte, maxCombinedSourceBytes+1))}}}
	source := globalSearchSource{native: native, dedicated: &appSearchFake{}}
	if _, err := source.Search(context.Background(), searchaction.Input{Terms: "sales", Limit: 20}); err == nil {
		t.Fatal("accepted an oversized nested continuation")
	}
}

func TestSearchCommandsUseNativeEndpointForContentTerms(t *testing.T) {
	searchCalls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/-/search":
			searchCalls++
			if request.URL.Query().Get("terms") != "Sales" || request.URL.Query().Get("filter") != "type:eq:workbook" {
				t.Errorf("native query=%q", request.URL.RawQuery)
			}
			_, _ = io.WriteString(writer, `{"items":[{"uri":"/workbooks/wb-1","content":{"type":"workbook","luid":"wb-1","title":"Sales Dashboard"}}],"limit":20,"pageIndex":0,"startIndex":0,"total":1}`)
		default:
			t.Errorf("unexpected legacy request %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: \"\"\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	code := Run(context.Background(), []string{"search", "Sales", "--type", "workbook", "--environment", "production"}, &stdout, Options{ConfigPath: configPath, HTTPClient: server.Client()})
	if code != 0 || searchCalls != 1 || !strings.Contains(stdout.String(), "wb-1,workbook,Sales Dashboard") || !strings.Contains(stdout.String(), "source: live") {
		t.Fatalf("code=%d calls=%d output=%s", code, searchCalls, stdout.String())
	}
}

func TestBlankTypedSearchUsesBoundedLivePagesWithoutCatalog(t *testing.T) {
	reads := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Sales"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/workbooks"):
			reads++
			number := r.URL.Query().Get("pageNumber")
			if r.URL.Query().Get("pageSize") != "1" {
				t.Errorf("unbounded request %s", r.URL.RawQuery)
			}
			id, name := "workbook-a", "Alpha"
			if number == "2" {
				id, name = "workbook-b", "Beta"
			}
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="1" totalAvailable="2"/><workbooks><workbook id="%s" name="%s"><project id="project-1"/></workbook></workbooks></tsResponse>`, number, id, name)
		default:
			http.Error(w, "unexpected request", 404)
		}
	}))
	defer server.Close()
	runtime := inventoryListRuntime(t, server)
	commands := newSearchCommands(runtime)
	input := searchaction.Input{Environment: "production", Type: "workbook", Limit: 1}
	first, err := commands.Execute(context.Background(), input)
	if err != nil || reads != 1 || len(first.Items) != 1 || first.Items[0].LUID != "workbook-a" || !first.Page.MoreAvailable {
		t.Fatalf("first=%+v reads=%d err=%v", first, reads, err)
	}
	input.Cursor = first.Page.NextCursor
	second, err := commands.Execute(context.Background(), input)
	if err != nil || reads != 2 || len(second.Items) != 1 || second.Items[0].LUID != "workbook-b" || second.Page.MoreAvailable {
		t.Fatalf("second=%+v reads=%d err=%v", second, reads, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(runtime.configPath), catalog.DatabasePath())); !os.IsNotExist(err) {
		t.Fatalf("limited search created catalog: %v", err)
	}
}
