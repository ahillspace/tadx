package definition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

func TestListOutputGolden(t *testing.T) {
	output := ListOutput{
		Status: "listed", Environment: "dev", Site: "sales",
		Page:        ListOutputPage{Returned: 1, Limit: 10, NextCursor: "next-page", MoreAvailable: true},
		Definitions: []ListDefinition{{LUID: "definition-1", Name: "Revenue", Description: "Recognized revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", Aggregation: "AGGREGATION_SUM", TimeDimension: "Order Date", AllowedDimensions: []string{"Region"}}},
		RequestID:   "request-1", Help: []string{"tadx pulse definition inspect --id <definition-luid>"}, Source: &readsource.Metadata{Mode: readsource.Tableau, ObservedAt: "2026-09-04T12:00:00Z", Coverage: readsource.CoverageComplete},
	}
	listAssertGolden(t, "compact.toon", output, false)
	listAssertGolden(t, "full.toon", output, true)
}

func listAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "list", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

type listReader struct {
	page  ListPage
	input ListPageRequest
	calls int
	err   error
	pages []ListPage
}

func (r *listReader) ListDefinitions(_ context.Context, input ListPageRequest) (ListPage, error) {
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
	r := &listReader{page: ListPage{
		Definitions:   []ListDefinition{{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales"}},
		NextPageToken: "provider-next", RequestID: "request-1",
	}}
	output, err := list(context.Background(), r, ListInput{Environment: "dev", Site: "sales", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.PageSize != 10 || r.input.PageToken != "" || output.Page.Returned != 1 || output.Page.NextCursor == "" || output.RequestID != "request-1" {
		t.Fatalf("request=%#v output=%#v", r.input, output)
	}
	compact := output.CompactOutput().(ListCompactResult)
	full := output.FullOutput().(ListFullResult)
	if compact.Definitions[0].LUID != "definition-1" || compact.Details != "--full" || full.Definitions[0].MeasureField != "Sales" {
		t.Fatalf("compact=%#v full=%#v", compact, full)
	}
}

func TestListContinuationBindsTargetAndLimit(t *testing.T) {
	firstReader := &listReader{page: ListPage{NextPageToken: "opaque", Definitions: []ListDefinition{}}}
	first, err := list(context.Background(), firstReader, ListInput{Environment: "dev", Site: "sales", Limit: 7})
	if err != nil {
		t.Fatal(err)
	}
	continuation := &listReader{page: ListPage{Definitions: []ListDefinition{}}}
	_, err = list(context.Background(), continuation, ListInput{Environment: "dev", Site: "sales", Limit: 7, Cursor: first.Page.NextCursor})
	if err != nil || continuation.input.PageToken != "opaque" {
		t.Fatalf("request=%#v err=%v", continuation.input, err)
	}
	_, err = list(context.Background(), &listReader{}, ListInput{Environment: "prod", Site: "sales", Limit: 7, Cursor: first.Page.NextCursor})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%#v", err)
	}
	_, err = list(context.Background(), &listReader{}, ListInput{Environment: "dev", Site: "sales", Limit: 7, Cursor: first.Page.NextCursor, Cache: true})
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("source-mismatched cursor error=%#v", err)
	}
}

func TestListRejectsInvalidLimitBeforeReader(t *testing.T) {
	r := &listReader{}
	_, err := list(context.Background(), r, ListInput{Limit: 10001})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || r.calls != 0 {
		t.Fatalf("error=%#v calls=%d", err, r.calls)
	}
}

func TestListAllCorrectionExplainsLimitChoice(t *testing.T) {
	_, err := list(context.Background(), &listReader{}, ListInput{All: true, Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "remove --limit") || !strings.Contains(err.Error(), "remove --all") {
		t.Fatalf("error=%v", err)
	}
}

func TestListNameFilterScansPagesAndBoundsMatchingResults(t *testing.T) {
	r := &listReader{pages: []ListPage{
		{Definitions: []ListDefinition{{LUID: "one", Name: "sales", DatasourceLUID: "ds"}}, NextPageToken: "second"},
		{Definitions: []ListDefinition{{LUID: "two", Name: "Sales", DatasourceLUID: "ds"}, {LUID: "three", Name: "Sales", DatasourceLUID: "ds"}}},
	}}
	out, err := list(context.Background(), r, ListInput{Name: "Sales", Limit: 1})
	if err != nil || r.calls != 2 || len(out.Definitions) != 1 || out.Definitions[0].LUID != "two" || !out.Page.MoreAvailable {
		t.Fatalf("out=%+v err=%v calls=%d", out, err, r.calls)
	}
}

func TestListAllRejectsBrokenPaginationAndIncompleteScan(t *testing.T) {
	for name, pages := range map[string][]ListPage{
		"repeat":             {{NextPageToken: "same"}, {NextPageToken: "same"}},
		"cycle":              {{NextPageToken: "a"}, {NextPageToken: "b"}, {NextPageToken: "a"}},
		"blank":              {{NextPageToken: " "}},
		"duplicate identity": {{Definitions: []ListDefinition{{LUID: "one", Name: "Sales", DatasourceLUID: "ds"}}, NextPageToken: "next"}, {Definitions: []ListDefinition{{LUID: "one", Name: "Sales", DatasourceLUID: "ds"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &listReader{pages: pages}
			if _, err := list(context.Background(), r, ListInput{All: true}); err == nil {
				t.Fatal("broken pagination accepted")
			}
		})
	}
	pages := make([]ListPage, 100)
	for index := range pages {
		pages[index].NextPageToken = fmt.Sprintf("page-%d", index)
	}
	r := &listReader{pages: pages}
	_, err := list(context.Background(), r, ListInput{All: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.definition.list.incomplete" || r.calls != 100 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func TestListAllRejectsExplicitLimitOrCursor(t *testing.T) {
	for _, input := range []ListInput{{All: true, Limit: 10}, {All: true, Cursor: "opaque"}} {
		r := &listReader{}
		if _, err := list(context.Background(), r, input); err == nil || r.calls != 0 {
			t.Fatalf("err=%v calls=%d", err, r.calls)
		}
	}
}

func TestListDatasourceFilterRetainsExactMatchAndCompletenessGuards(t *testing.T) {
	for name, pages := range map[string][]ListPage{
		"repeated token":   {{NextPageToken: "same"}, {NextPageToken: "same"}},
		"missing identity": {{Definitions: []ListDefinition{{LUID: "one", Name: "Revenue"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &listReader{pages: pages}
			if _, err := list(context.Background(), r, ListInput{DatasourceLUID: "target", Limit: 1}); err == nil {
				t.Fatal("incomplete filtered scan accepted")
			}
		})
	}
	pages := make([]ListPage, 100)
	for i := range pages {
		pages[i] = ListPage{Definitions: []ListDefinition{{LUID: fmt.Sprint(i), Name: "Revenue", DatasourceLUID: "unrelated"}}, NextPageToken: fmt.Sprintf("next-%d", i)}
	}
	r := &listReader{pages: pages}
	_, err := list(context.Background(), r, ListInput{DatasourceLUID: "target", Limit: 1})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.definition.list.incomplete" || r.calls != 100 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
	r = &listReader{pages: []ListPage{{Definitions: []ListDefinition{{LUID: "one", Name: "Revenue", DatasourceLUID: "TARGET"}}}, {}}}
	out, err := list(context.Background(), r, ListInput{DatasourceLUID: "target", All: true})
	if err != nil || len(out.Definitions) != 0 || out.Page.MoreAvailable {
		t.Fatalf("exact case-sensitive filter: out=%#v err=%v", out, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = &listReader{}
	if _, err := list(ctx, r, ListInput{DatasourceLUID: "target"}); !errors.Is(err, context.Canceled) || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}
