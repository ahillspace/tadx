package metric

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

type listReader struct{ request ListPageRequest }

type listPageReader struct {
	pages []ListPage
	calls int
}

func (r *listPageReader) ListMetrics(_ context.Context, _ string, _ ListPageRequest) (ListPage, error) {
	if r.calls >= len(r.pages) {
		return ListPage{}, errors.New("unexpected page")
	}
	page := r.pages[r.calls]
	r.calls++
	return page, nil
}
func TestListAllRejectsBrokenPaginationAndScanOverflow(t *testing.T) {
	for name, pages := range map[string][]ListPage{
		"repeat":    {{NextPageToken: "same"}, {NextPageToken: "same"}},
		"cycle":     {{NextPageToken: "a"}, {NextPageToken: "b"}, {NextPageToken: "a"}},
		"blank":     {{NextPageToken: " "}},
		"duplicate": {{Metrics: []ListMetric{{LUID: "one", DefinitionLUID: "def"}}, NextPageToken: "next"}, {Metrics: []ListMetric{{LUID: "one", DefinitionLUID: "def"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &listPageReader{pages: pages}
			if _, err := list(context.Background(), r, ListInput{DefinitionLUID: "def", All: true}); err == nil {
				t.Fatal("broken pagination accepted")
			}
		})
	}
	pages := make([]ListPage, 100)
	for index := range pages {
		pages[index].NextPageToken = fmt.Sprintf("next-%d", index)
	}
	r := &listPageReader{pages: pages}
	_, err := list(context.Background(), r, ListInput{DefinitionLUID: "def", All: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.metric.list.incomplete" || r.calls != 100 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func (r *listReader) ListMetrics(_ context.Context, definition string, request ListPageRequest) (ListPage, error) {
	r.request = request
	return ListPage{Metrics: []ListMetric{{LUID: "metric-1", Name: "Revenue", DefinitionLUID: definition}}, NextPageToken: "next", RequestID: "request-1"}, nil
}

func TestListReturnsOneBoundedDefinitionPage(t *testing.T) {
	r := &listReader{}
	output, err := list(context.Background(), r, ListInput{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if output.Page.Returned != 1 || output.Metrics[0].DefinitionLUID != "definition-1" || output.Page.NextCursor == "" || r.request.PageSize != 10 {
		t.Fatalf("output=%#v request=%#v", output, r.request)
	}
}

func TestListOutputGolden(t *testing.T) {
	source := readsource.Live(time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC))
	output := ListOutput{
		Status: "listed", Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1",
		Page: ListOutputPage{Returned: 2, Limit: 20, NextCursor: "cursor-2", MoreAvailable: true},
		Metrics: []ListMetric{
			{LUID: "metric-1", Name: "Revenue", DefinitionLUID: "definition-1", IsDefault: true},
			{LUID: "metric-2", Name: "Revenue West", DefinitionLUID: "definition-1"},
		},
		RequestID: "request-1",
		Source:    &source,
		Help:      []string{"tadx pulse metric inspect --id metric-1"},
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

func TestListRejectsMissingDefinition(t *testing.T) {
	if _, err := list(context.Background(), &listReader{}, ListInput{}); err == nil {
		t.Fatal("missing definition accepted")
	}
}

func TestListAllCorrectionExplainsLimitChoice(t *testing.T) {
	_, err := list(context.Background(), &listReader{}, ListInput{DefinitionLUID: "definition-1", All: true, Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "remove --limit") || !strings.Contains(err.Error(), "remove --all") {
		t.Fatalf("error=%v", err)
	}
}

func TestListCursorCannotSwitchBetweenTableauAndCache(t *testing.T) {
	r := &listReader{}
	output, err := list(context.Background(), r, ListInput{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list(context.Background(), r, ListInput{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10, Cursor: output.Page.NextCursor, Cache: true}); err == nil {
		t.Fatal("cursor accepted after source switch")
	}
	if _, err := list(context.Background(), r, ListInput{Environment: "prod", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10, Cursor: output.Page.NextCursor}); err == nil {
		t.Fatal("cursor accepted after environment switch")
	}
}
