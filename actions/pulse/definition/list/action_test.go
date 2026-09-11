package list_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

func TestOutputGolden(t *testing.T) {
	output := definitionlist.Output{
		Status: "listed", Environment: "dev", Site: "sales",
		Page:        definitionlist.OutputPage{Returned: 1, Limit: 10, NextCursor: "next-page", MoreAvailable: true},
		Definitions: []definitionlist.Definition{{LUID: "definition-1", Name: "Revenue", Description: "Recognized revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", Aggregation: "AGGREGATION_SUM", TimeDimension: "Order Date", AllowedDimensions: []string{"Region"}}},
		RequestID:   "request-1", Help: []string{"tadx pulse definition inspect --id <definition-luid>"}, Source: &readsource.Metadata{Mode: readsource.Tableau, ObservedAt: "2026-09-04T12:00:00Z", Coverage: readsource.CoverageComplete},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

type reader struct {
	page  definitionlist.Page
	input definitionlist.PageRequest
	calls int
	err   error
	pages []definitionlist.Page
}

func (r *reader) ListDefinitions(_ context.Context, input definitionlist.PageRequest) (definitionlist.Page, error) {
	r.calls++
	r.input = input
	if len(r.pages) > 0 {
		page := r.pages[0]
		r.pages = r.pages[1:]
		return page, r.err
	}
	return r.page, r.err
}

func TestListReturnsBoundedDefinitionPage(t *testing.T) {
	r := &reader{page: definitionlist.Page{
		Definitions:   []definitionlist.Definition{{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales"}},
		NextPageToken: "provider-next", RequestID: "request-1",
	}}
	output, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{Environment: "dev", Site: "sales", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.PageSize != 10 || r.input.PageToken != "" || output.Page.Returned != 1 || output.Page.NextCursor == "" || output.RequestID != "request-1" {
		t.Fatalf("request=%#v output=%#v", r.input, output)
	}
	compact := output.CompactOutput().(definitionlist.CompactResult)
	full := output.FullOutput().(definitionlist.FullResult)
	if compact.Definitions[0].LUID != "definition-1" || compact.Details != "--full" || full.Definitions[0].MeasureField != "Sales" {
		t.Fatalf("compact=%#v full=%#v", compact, full)
	}
}

func TestListContinuationBindsTargetAndLimit(t *testing.T) {
	firstReader := &reader{page: definitionlist.Page{NextPageToken: "opaque", Definitions: []definitionlist.Definition{}}}
	first, err := definitionlist.New(firstReader).Execute(context.Background(), definitionlist.Input{Environment: "dev", Site: "sales", Limit: 7})
	if err != nil {
		t.Fatal(err)
	}
	continuation := &reader{page: definitionlist.Page{Definitions: []definitionlist.Definition{}}}
	_, err = definitionlist.New(continuation).Execute(context.Background(), definitionlist.Input{Environment: "dev", Site: "sales", Limit: 7, Cursor: first.Page.NextCursor})
	if err != nil || continuation.input.PageToken != "opaque" {
		t.Fatalf("request=%#v err=%v", continuation.input, err)
	}
	_, err = definitionlist.New(&reader{}).Execute(context.Background(), definitionlist.Input{Environment: "prod", Site: "sales", Limit: 7, Cursor: first.Page.NextCursor})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%#v", err)
	}
	_, err = definitionlist.New(&reader{}).Execute(context.Background(), definitionlist.Input{Environment: "dev", Site: "sales", Limit: 7, Cursor: first.Page.NextCursor, Cache: true})
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("source-mismatched cursor error=%#v", err)
	}
}

func TestListRejectsInvalidLimitBeforeReader(t *testing.T) {
	r := &reader{}
	_, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{Limit: 10001})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || r.calls != 0 {
		t.Fatalf("error=%#v calls=%d", err, r.calls)
	}
}

func TestListNameFilterScansPagesAndBoundsMatchingResults(t *testing.T) {
	r := &reader{pages: []definitionlist.Page{
		{Definitions: []definitionlist.Definition{{LUID: "one", Name: "sales", DatasourceLUID: "ds"}}, NextPageToken: "second"},
		{Definitions: []definitionlist.Definition{{LUID: "two", Name: "Sales", DatasourceLUID: "ds"}, {LUID: "three", Name: "Sales", DatasourceLUID: "ds"}}},
	}}
	out, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{Name: "Sales", Limit: 1})
	if err != nil || r.calls != 2 || len(out.Definitions) != 1 || out.Definitions[0].LUID != "two" || !out.Page.MoreAvailable {
		t.Fatalf("out=%+v err=%v calls=%d", out, err, r.calls)
	}
}

func TestListAllRejectsBrokenPaginationAndIncompleteScan(t *testing.T) {
	for name, pages := range map[string][]definitionlist.Page{
		"repeat":             {{NextPageToken: "same"}, {NextPageToken: "same"}},
		"cycle":              {{NextPageToken: "a"}, {NextPageToken: "b"}, {NextPageToken: "a"}},
		"blank":              {{NextPageToken: " "}},
		"duplicate identity": {{Definitions: []definitionlist.Definition{{LUID: "one", Name: "Sales", DatasourceLUID: "ds"}}, NextPageToken: "next"}, {Definitions: []definitionlist.Definition{{LUID: "one", Name: "Sales", DatasourceLUID: "ds"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &reader{pages: pages}
			if _, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{All: true}); err == nil {
				t.Fatal("broken pagination accepted")
			}
		})
	}
	pages := make([]definitionlist.Page, 100)
	for index := range pages {
		pages[index].NextPageToken = fmt.Sprintf("page-%d", index)
	}
	r := &reader{pages: pages}
	_, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{All: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.definition.list.incomplete" || r.calls != 100 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func TestListAllRejectsExplicitLimitOrCursor(t *testing.T) {
	for _, input := range []definitionlist.Input{{All: true, Limit: 10}, {All: true, Cursor: "opaque"}} {
		r := &reader{}
		if _, err := definitionlist.New(r).Execute(context.Background(), input); err == nil || r.calls != 0 {
			t.Fatalf("err=%v calls=%d", err, r.calls)
		}
	}
}

func TestDatasourceFilterRetainsExactMatchAndCompletenessGuards(t *testing.T) {
	for name, pages := range map[string][]definitionlist.Page{
		"repeated token":   {{NextPageToken: "same"}, {NextPageToken: "same"}},
		"missing identity": {{Definitions: []definitionlist.Definition{{LUID: "one", Name: "Revenue"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &reader{pages: pages}
			if _, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{DatasourceLUID: "target", Limit: 1}); err == nil {
				t.Fatal("incomplete filtered scan accepted")
			}
		})
	}
	pages := make([]definitionlist.Page, 100)
	for i := range pages {
		pages[i] = definitionlist.Page{Definitions: []definitionlist.Definition{{LUID: fmt.Sprint(i), Name: "Revenue", DatasourceLUID: "unrelated"}}, NextPageToken: fmt.Sprintf("next-%d", i)}
	}
	r := &reader{pages: pages}
	_, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{DatasourceLUID: "target", Limit: 1})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.definition.list.incomplete" || r.calls != 100 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
	r = &reader{pages: []definitionlist.Page{{Definitions: []definitionlist.Definition{{LUID: "one", Name: "Revenue", DatasourceLUID: "TARGET"}}}, {}}}
	out, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{DatasourceLUID: "target", All: true})
	if err != nil || len(out.Definitions) != 0 || out.Page.MoreAvailable {
		t.Fatalf("exact case-sensitive filter: out=%#v err=%v", out, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = &reader{}
	if _, err := definitionlist.New(r).Execute(ctx, definitionlist.Input{DatasourceLUID: "target"}); !errors.Is(err, context.Canceled) || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}
