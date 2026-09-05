package list_test

import (
	"bytes"
	"context"
	"errors"
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
		Page:        definitionlist.OutputPage{Returned: 1, Limit: 10, NextCursor: "next-page"},
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
}

func (r *reader) ListDefinitions(_ context.Context, input definitionlist.PageRequest) (definitionlist.Page, error) {
	r.calls++
	r.input = input
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
	_, err = definitionlist.New(&reader{}).Execute(context.Background(), definitionlist.Input{Environment: "dev", Site: "sales", Limit: 7, Cursor: first.Page.NextCursor, Catalog: true})
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("source-mismatched cursor error=%#v", err)
	}
}

func TestListRejectsInvalidLimitBeforeReader(t *testing.T) {
	r := &reader{}
	_, err := definitionlist.New(r).Execute(context.Background(), definitionlist.Input{Limit: 101})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || r.calls != 0 {
		t.Fatalf("error=%#v calls=%d", err, r.calls)
	}
}

func TestListNameFilterIsExactAndPreservesProviderContinuation(t *testing.T) {
	r := &reader{page: definitionlist.Page{Definitions: []definitionlist.Definition{
		{LUID: "one", Name: "Sales", DatasourceLUID: "ds"},
		{LUID: "two", Name: "sales", DatasourceLUID: "ds"},
		{LUID: "three", Name: "Sales Target", DatasourceLUID: "ds"},
	}, NextPageToken: "second-page"}}
	in := definitionlist.Input{Environment: "dev", Site: "site", Name: "Sales", Limit: 3}
	out, err := definitionlist.New(r).Execute(context.Background(), in)
	if err != nil || r.calls != 1 || r.input.PageSize != 3 || out.Page.Returned != 1 || len(out.Definitions) != 1 || out.Definitions[0].LUID != "one" || out.Page.NextCursor == "" {
		t.Fatalf("out=%+v err=%v reader=%+v", out, err, r)
	}
	if len(r.page.Definitions) != 3 || r.page.Definitions[1].LUID != "two" {
		t.Fatal("filter mutated the provider page")
	}
	in.Cursor = out.Page.NextCursor
	continuation := &reader{page: definitionlist.Page{Definitions: []definitionlist.Definition{{LUID: "four", Name: "Other", DatasourceLUID: "ds"}}, NextPageToken: "third-page"}}
	out, err = definitionlist.New(continuation).Execute(context.Background(), in)
	if err != nil || continuation.calls != 1 || continuation.input.PageToken != "second-page" || out.Page.Returned != 0 || out.Page.NextCursor == "" {
		t.Fatalf("empty filtered page must preserve continuation: out=%+v err=%v reader=%+v", out, err, continuation)
	}
	for _, name := range []string{"", "sales", "Sales Target"} {
		in.Name = name
		r := &reader{}
		_, err := definitionlist.New(r).Execute(context.Background(), in)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || r.calls != 0 {
			t.Fatalf("changed name %q accepted cursor: err=%v calls=%d", name, err, r.calls)
		}
	}
}
