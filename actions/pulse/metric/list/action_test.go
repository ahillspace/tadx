package list_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	metriclist "github.com/ahillspace/tadx/actions/pulse/metric/list"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

type reader struct{ request metriclist.PageRequest }

type pageReader struct {
	pages []metriclist.Page
	calls int
}

func (r *pageReader) ListMetrics(_ context.Context, _ string, _ metriclist.PageRequest) (metriclist.Page, error) {
	if r.calls >= len(r.pages) {
		return metriclist.Page{}, errors.New("unexpected page")
	}
	page := r.pages[r.calls]
	r.calls++
	return page, nil
}
func TestListAllRejectsBrokenPaginationAndScanOverflow(t *testing.T) {
	for name, pages := range map[string][]metriclist.Page{
		"repeat":    {{NextPageToken: "same"}, {NextPageToken: "same"}},
		"cycle":     {{NextPageToken: "a"}, {NextPageToken: "b"}, {NextPageToken: "a"}},
		"blank":     {{NextPageToken: " "}},
		"duplicate": {{Metrics: []metriclist.Metric{{LUID: "one", DefinitionLUID: "def"}}, NextPageToken: "next"}, {Metrics: []metriclist.Metric{{LUID: "one", DefinitionLUID: "def"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &pageReader{pages: pages}
			if _, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{DefinitionLUID: "def", All: true}); err == nil {
				t.Fatal("broken pagination accepted")
			}
		})
	}
	pages := make([]metriclist.Page, 100)
	for index := range pages {
		pages[index].NextPageToken = fmt.Sprintf("next-%d", index)
	}
	r := &pageReader{pages: pages}
	_, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{DefinitionLUID: "def", All: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.metric.list.incomplete" || r.calls != 100 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func (r *reader) ListMetrics(_ context.Context, definition string, request metriclist.PageRequest) (metriclist.Page, error) {
	r.request = request
	return metriclist.Page{Metrics: []metriclist.Metric{{LUID: "metric-1", Name: "Revenue", DefinitionLUID: definition}}, NextPageToken: "next", RequestID: "request-1"}, nil
}

func TestListReturnsOneBoundedDefinitionPage(t *testing.T) {
	r := &reader{}
	output, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if output.Page.Returned != 1 || output.Metrics[0].DefinitionLUID != "definition-1" || output.Page.NextCursor == "" || r.request.PageSize != 10 {
		t.Fatalf("output=%#v request=%#v", output, r.request)
	}
}

func TestListOutputGolden(t *testing.T) {
	source := readsource.Live(time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC))
	output := metriclist.Output{
		Status: "listed", Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1",
		Page: metriclist.OutputPage{Returned: 2, Limit: 20, NextCursor: "cursor-2", MoreAvailable: true},
		Metrics: []metriclist.Metric{
			{LUID: "metric-1", Name: "Revenue", DefinitionLUID: "definition-1", IsDefault: true},
			{LUID: "metric-2", Name: "Revenue West", DefinitionLUID: "definition-1"},
		},
		RequestID: "request-1",
		Source:    &source,
		Help:      []string{"tadx pulse metric inspect --id metric-1"},
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

func TestListRejectsMissingDefinition(t *testing.T) {
	if _, err := metriclist.New(&reader{}).Execute(context.Background(), metriclist.Input{}); err == nil {
		t.Fatal("missing definition accepted")
	}
}

func TestCursorCannotSwitchBetweenTableauAndCatalog(t *testing.T) {
	r := &reader{}
	output, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10, Cursor: output.Page.NextCursor, Catalog: true}); err == nil {
		t.Fatal("cursor accepted after source switch")
	}
	if _, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "prod", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10, Cursor: output.Page.NextCursor}); err == nil {
		t.Fatal("cursor accepted after environment switch")
	}
}
