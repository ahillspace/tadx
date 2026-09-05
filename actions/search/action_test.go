package search_test

import (
	"bytes"
	"context"
	"errors"
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
	s := &source{result: search.Result{Source: "catalog", Items: []search.Item{{LUID: "wb-1", Type: "workbook", Name: "Finance"}}}}
	out, err := search.New(s).Execute(context.Background(), search.Input{Type: "workbook"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "catalog" {
		t.Fatalf("source=%q", out.Source)
	}
}

type source struct {
	inputs []search.Input
	result search.Result
	err    error
}

func (s *source) Search(_ context.Context, input search.Input) (search.Result, error) {
	s.inputs = append(s.inputs, input)
	return s.result, s.err
}

func TestSearchValidatesSelectorsBeforeCallingSource(t *testing.T) {
	for _, input := range []search.Input{{}, {Terms: " "}, {Terms: "sales", Type: "views"}, {Terms: "sales", Limit: 101}, {Terms: "sales", Limit: -1}, {Terms: "sales", Environment: "production"}} {
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
		"catalog": func(i *search.Input) { i.Catalog = true },
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

func TestCatalogSearchAlwaysQualifiesAbsence(t *testing.T) {
	s := &source{result: search.Result{Generation: &search.Generation{ID: "generation-1"}}}
	out, err := search.New(s).Execute(context.Background(), search.Input{Terms: "absent", Catalog: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "catalog" || out.Generation == nil || !strings.Contains(strings.Join(out.Warnings, " "), "absence does not establish remote absence") {
		t.Fatalf("output=%+v", out)
	}
	if !reflect.DeepEqual(out.Items, []search.Item{}) {
		t.Fatalf("items=%#v", out.Items)
	}
}

type unavailableCatalogScope struct{}

func (unavailableCatalogScope) Error() string                 { return "scope unavailable" }
func (unavailableCatalogScope) CatalogScopeUnavailable() bool { return true }

func TestCatalogSearchRejectsUnavailableTypeExplicitly(t *testing.T) {
	_, err := search.New(&source{err: unavailableCatalogScope{}}).Execute(context.Background(), search.Input{Type: "metric", Catalog: true})
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
