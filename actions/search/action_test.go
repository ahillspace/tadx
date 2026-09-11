package search_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

func TestSearchCompactAndFullOutput(t *testing.T) {
	for _, full := range []bool{false, true} {
		s := &source{result: search.Result{Items: []search.Item{{LUID: "wb-1", Type: "workbook", Name: "Finance", ProjectPath: "Ops", Owner: "owner-1", ModifiedAt: "2026-09-01T10:00:00Z"}}}}
		out, err := search.New(s).Execute(context.Background(), search.Input{Terms: "Finance"})
		if err != nil {
			t.Fatal(err)
		}
		var actual bytes.Buffer
		if err := output.RenderWithOptions(&actual, out, output.Options{Full: full}); err != nil {
			t.Fatal(err)
		}
		path := "testdata/compact.toon"
		if full {
			path = "testdata/full.toon"
		}
		expected, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual.Bytes(), expected) {
			t.Errorf("%s mismatch:\n%s", path, actual.String())
		}
	}
}

func TestSearchPreservesSourceReportedByComposedListContinuation(t *testing.T) {
	s := &source{result: search.Result{Source: "cache", Items: []search.Item{{LUID: "wb-1", Type: "workbook", Name: "Finance"}}}}
	out, err := search.New(s).Execute(context.Background(), search.Input{Type: "workbook"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "cache" {
		t.Fatalf("source=%q", out.Source)
	}
}

type source struct {
	inputs []search.Input
	result search.Result
	err    error
}

type pageSource struct {
	pages []search.Result
	calls int
}

func (s *pageSource) Search(_ context.Context, input search.Input) (search.Result, error) {
	if s.calls >= len(s.pages) {
		return search.Result{}, errors.New("unexpected page")
	}
	if input.Limit > 100 {
		return search.Result{}, errors.New("provider limit exceeded")
	}
	page := s.pages[s.calls]
	s.calls++
	return page, nil
}
func TestExpandedSearchRejectsCyclesAndScanOverflow(t *testing.T) {
	for name, pages := range map[string][]search.Result{
		"repeat":    {{Page: search.Page{NextCursor: "same"}}, {Page: search.Page{NextCursor: "same"}}},
		"cycle":     {{Page: search.Page{NextCursor: "a"}}, {Page: search.Page{NextCursor: "b"}}, {Page: search.Page{NextCursor: "a"}}},
		"blank":     {{Page: search.Page{NextCursor: " "}}},
		"duplicate": {{Items: []search.Item{{LUID: "one", Name: "A", Type: "workbook"}}, Page: search.Page{NextCursor: "next"}}, {Items: []search.Item{{LUID: "one", Name: "A", Type: "workbook"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			s := &pageSource{pages: pages}
			if _, err := search.New(s).Execute(context.Background(), search.Input{Type: "workbook", Limit: 200}); err == nil {
				t.Fatal("broken pagination accepted")
			}
		})
	}
	pages := make([]search.Result, 100)
	for index := range pages {
		pages[index].Page.NextCursor = fmt.Sprintf("next-%d", index)
	}
	s := &pageSource{pages: pages}
	if _, err := search.New(s).Execute(context.Background(), search.Input{Type: "workbook", Limit: 200}); err == nil || s.calls != 100 {
		t.Fatalf("err=%v calls=%d", err, s.calls)
	}
}

func TestSearchUnsupportedTypeHasConcreteCorrection(t *testing.T) {
	for _, kind := range []string{"all", "pulse_definition", "datasources", "view"} {
		_, err := search.New(&source{}).Execute(context.Background(), search.Input{Type: kind})
		var structured *errs.Error
		if !errors.As(err, &structured) || !strings.Contains(structured.CorrectiveAction, "omit --type") || !strings.Contains(structured.CorrectiveAction, "definition, metric") {
			t.Fatalf("kind=%s err=%v", kind, err)
		}
	}
}

func (s *source) Search(_ context.Context, input search.Input) (search.Result, error) {
	s.inputs = append(s.inputs, input)
	return s.result, s.err
}

func TestSearchValidatesSelectorsBeforeCallingSource(t *testing.T) {
	for _, input := range []search.Input{{}, {Terms: " "}, {Terms: "sales", Type: "views"}, {Terms: "sales", Limit: 2001}, {Terms: "sales", Limit: -1}, {Terms: "sales", Environment: "production"}} {
		s := &source{}
		_, err := search.New(s).Execute(context.Background(), input)
		var typed *errs.Error
		if !errors.As(err, &typed) || typed.Kind != errs.KindUsage || len(s.inputs) != 0 {
			t.Fatalf("input=%+v error=%v calls=%d", input, err, len(s.inputs))
		}
	}
}

func TestSearchAcceptsEachTypeAndNormalizesDefaultLimit(t *testing.T) {
	for _, kind := range []string{"content", "admin", "pulse", "workbook", "datasource", "flow", "project", "user", "group", "definition", "metric"} {
		s := &source{}
		out, err := search.New(s).Execute(context.Background(), search.Input{Type: kind})
		if err != nil {
			t.Fatal(err)
		}
		if len(s.inputs) != 1 || s.inputs[0].Limit != 20 || out.Items == nil || out.Page.Limit != 20 {
			t.Fatalf("type=%s inputs=%+v output=%+v", kind, s.inputs, out)
		}
	}
}

func TestSearchFamilyTypes(t *testing.T) {
	for selector, want := range map[string][]string{"content": {"datasource", "flow", "project", "workbook"}, "admin": {"group", "user"}, "pulse": {"definition", "metric"}} {
		got, err := search.Types(selector)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s types=%v error=%v", selector, got, err)
		}
	}
}

func TestSearchRejectsUnboundedOrInvalidIdentityResults(t *testing.T) {
	for _, result := range []search.Result{
		{Items: []search.Item{{LUID: "1", Type: "workbook", Name: "A"}, {LUID: "2", Type: "workbook", Name: "B"}}},
		{Items: []search.Item{{Type: "workbook", Name: "A"}}},
		{Items: []search.Item{{LUID: "1", Type: "view", Name: "A"}}},
		{Items: []search.Item{{LUID: "1", Type: "user", Name: "A"}}},
	} {
		_, err := search.New(&source{result: result}).Execute(context.Background(), search.Input{Terms: "a", Type: "workbook", Limit: 1})
		if err == nil {
			t.Fatalf("accepted %+v", result)
		}
	}
}

func TestSearchBindsCursorToSourceAndEveryFilter(t *testing.T) {
	base := search.Input{Terms: "sales", Type: "workbook", Environment: "production", Site: "site-1", SiteResolved: true, Owner: "owner", ProjectPath: "Ops", Limit: 1}
	out, err := search.New(&source{result: search.Result{Page: search.Page{NextCursor: "next"}}}).Execute(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*search.Input){
		"terms": func(i *search.Input) { i.Terms = "finance" }, "type": func(i *search.Input) { i.Type = "content" },
		"environment": func(i *search.Input) { i.Environment = "staging" }, "site": func(i *search.Input) { i.Site = "site-2" },
		"owner":   func(i *search.Input) { i.Owner = "other" },
		"project": func(i *search.Input) { i.ProjectPath = "Sales" }, "limit": func(i *search.Input) { i.Limit = 2 },
		"cache": func(i *search.Input) { i.Cache = true },
	} {
		t.Run(name, func(t *testing.T) {
			in := base
			in.Cursor = out.Page.NextCursor
			change(&in)
			s := &source{}
			if _, err := search.New(s).Execute(context.Background(), in); err == nil || len(s.inputs) != 0 {
				t.Fatalf("error=%v inputs=%+v", err, s.inputs)
			}
		})
	}
	base.Cursor = out.Page.NextCursor
	s := &source{}
	if _, err := search.New(s).Execute(context.Background(), base); err != nil || s.inputs[0].Cursor != "next" {
		t.Fatalf("error=%v inputs=%+v", err, s.inputs)
	}
}

func TestCacheSearchAlwaysQualifiesAbsence(t *testing.T) {
	s := &source{result: search.Result{Generation: &search.Generation{ID: "generation-1"}}}
	out, err := search.New(s).Execute(context.Background(), search.Input{Terms: "absent", Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "cache" || out.Generation == nil || !strings.Contains(strings.Join(out.Warnings, " "), "absence does not establish remote absence") {
		t.Fatalf("output=%+v", out)
	}
	if !reflect.DeepEqual(out.Items, []search.Item{}) {
		t.Fatalf("items=%#v", out.Items)
	}
}

type unavailableCacheScope struct{}

func (unavailableCacheScope) Error() string               { return "scope unavailable" }
func (unavailableCacheScope) CacheScopeUnavailable() bool { return true }

func TestCacheSearchRejectsUnavailableTypeExplicitly(t *testing.T) {
	_, err := search.New(&source{err: unavailableCacheScope{}}).Execute(context.Background(), search.Input{Type: "metric", Cache: true})
	var typed *errs.Error
	if !errors.As(err, &typed) || typed.Kind != errs.KindUsage || !strings.Contains(typed.Summary, "not available") {
		t.Fatalf("error=%v", err)
	}
}

func TestSearchAcceptsMetricWithoutDisplayName(t *testing.T) {
	item := search.Item{LUID: "metric-1", Type: "metric"}
	out, err := search.New(&source{result: search.Result{Items: []search.Item{item}}}).Execute(context.Background(), search.Input{Type: "metric"})
	if err != nil || !reflect.DeepEqual(out.Items, []search.Item{item}) {
		t.Fatalf("output=%+v error=%v", out, err)
	}
}

func TestSearchPreservesAdapterOrderAcrossPages(t *testing.T) {
	items := []search.Item{{LUID: "2", Type: "workbook", Name: "B"}, {LUID: "1", Type: "workbook", Name: "A"}}
	out, err := search.New(&source{result: search.Result{Items: items}}).Execute(context.Background(), search.Input{Terms: "x"})
	if err != nil || !reflect.DeepEqual(out.Items, items) {
		t.Fatalf("output=%+v error=%v", out, err)
	}
}
