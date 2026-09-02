package search_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	search "github.com/ahillspace/tadx/actions/content/search"
	"github.com/ahillspace/tadx/internal/output"
)

type source struct{ result search.Result }

func (s source) Search(context.Context, search.Input) (search.Result, error) { return s.result, nil }

type recordingSource struct {
	inputs []search.Input
	result search.Result
}

func (s *recordingSource) Search(_ context.Context, input search.Input) (search.Result, error) {
	s.inputs = append(s.inputs, input)
	return s.result, nil
}

func TestActionCompactAndFullOutput(t *testing.T) {
	action := search.New(source{result: search.Result{Page: search.Page{Returned: 1, Limit: 1, NextCursor: "next"}, Items: []search.Item{{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops", Owner: "alice", ModifiedAt: "2026-09-01T10:00:00Z"}}, Warnings: []string{"results reflect current remote visibility"}}})
	for _, test := range []struct {
		name, golden string
		full         bool
	}{{"compact", "testdata/compact.toon", false}, {"full", "testdata/full.toon", true}} {
		t.Run(test.name, func(t *testing.T) {
			value, err := action.Execute(context.Background(), search.Input{Terms: "Finance", Kinds: []string{"workbook"}, Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			assertGolden(t, test.golden, value, test.full)
		})
	}
}

func TestActionBindsContinuationTokenToEveryFilter(t *testing.T) {
	base := search.Input{
		Environment: "production", Site: "marketing", SiteResolved: true, Terms: "Finance",
		Kinds: []string{"workbook", "datasource"}, OwnerLUID: "user-1", ProjectLUID: "project-1",
		ModifiedAfter: "2026-08-01T00:00:00Z", ModifiedBefore: "2026-09-01T00:00:00Z", Limit: 1,
	}
	firstSource := &recordingSource{result: search.Result{Page: search.Page{NextCursor: "upstream-next"}}}
	first, err := search.New(firstSource).Execute(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if first.Page.NextCursor == "" || first.Page.NextCursor == "upstream-next" {
		t.Fatalf("next cursor = %q", first.Page.NextCursor)
	}
	changes := map[string]func(*search.Input){
		"environment":     func(value *search.Input) { value.Environment = "staging" },
		"site":            func(value *search.Input) { value.Site = "sales" },
		"terms":           func(value *search.Input) { value.Terms = "Revenue" },
		"kinds":           func(value *search.Input) { value.Kinds = []string{"workbook"} },
		"owner":           func(value *search.Input) { value.OwnerLUID = "user-2" },
		"project":         func(value *search.Input) { value.ProjectLUID = "project-2" },
		"modified after":  func(value *search.Input) { value.ModifiedAfter = "2026-08-02T00:00:00Z" },
		"modified before": func(value *search.Input) { value.ModifiedBefore = "2026-08-31T00:00:00Z" },
		"limit":           func(value *search.Input) { value.Limit = 2 },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			input := base
			input.Kinds = append([]string(nil), base.Kinds...)
			input.Cursor = first.Page.NextCursor
			change(&input)
			nextSource := &recordingSource{}
			_, err := search.New(nextSource).Execute(context.Background(), input)
			if err == nil || len(nextSource.inputs) != 0 {
				t.Fatalf("Execute() error = %v, source calls = %d", err, len(nextSource.inputs))
			}
		})
	}
	continuedSource := &recordingSource{}
	continued := base
	continued.Cursor = first.Page.NextCursor
	if _, err := search.New(continuedSource).Execute(context.Background(), continued); err != nil {
		t.Fatal(err)
	}
	if len(continuedSource.inputs) != 1 || continuedSource.inputs[0].Cursor != "upstream-next" {
		t.Fatalf("source inputs = %#v", continuedSource.inputs)
	}
}

func TestActionBoundsWarningsAndPreservesEmptyItems(t *testing.T) {
	warnings := make([]string, 25)
	for index := range warnings {
		warnings[index] = strings.Repeat("warning", 100)
	}
	result, err := search.New(source{result: search.Result{Warnings: warnings}}).Execute(context.Background(), search.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || len(result.Warnings) != 20 || len([]rune(result.Warnings[0])) != 512 || !strings.HasSuffix(result.Warnings[0], "...") {
		t.Fatalf("output bounds = items:%#v warnings:%d first runes:%d", result.Items, len(result.Warnings), len([]rune(result.Warnings[0])))
	}
}

type pagingSource struct {
	page1, page2 search.Result
	cursors      []string
}

func (s *pagingSource) Search(_ context.Context, input search.Input) (search.Result, error) {
	s.cursors = append(s.cursors, input.Cursor)
	if input.Cursor == "" {
		return s.page1, nil
	}
	return s.page2, nil
}

// TestActionPreservesUpstreamOrderAcrossPages proves the action does not re-sort
// pages locally: the upstream cursor owns global ordering, so concatenating
// page 1 then page 2 must reproduce the upstream order verbatim (monotonic).
func TestActionPreservesUpstreamOrderAcrossPages(t *testing.T) {
	src := &pagingSource{
		page1: search.Result{
			Page:  search.Page{Returned: 2, Limit: 2, NextCursor: "upstream-next"},
			Items: []search.Item{{LUID: "1", Kind: "workbook", Name: "Alpha"}, {LUID: "2", Kind: "datasource", Name: "Beta"}},
		},
		page2: search.Result{
			Page:  search.Page{Returned: 2, Limit: 2},
			Items: []search.Item{{LUID: "3", Kind: "workbook", Name: "Gamma"}, {LUID: "4", Kind: "datasource", Name: "Delta"}},
		},
	}
	base := search.Input{Terms: "x", Limit: 2}
	first, err := search.New(src).Execute(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	// Local (Kind,Name) sorting would have reordered page 1 to [datasource/Beta, workbook/Alpha].
	if first.Items[0].LUID != "1" || first.Items[1].LUID != "2" {
		t.Fatalf("page 1 order not preserved: %#v", first.Items)
	}
	next := base
	next.Cursor = first.Page.NextCursor
	second, err := search.New(src).Execute(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if second.Items[0].LUID != "3" || second.Items[1].LUID != "4" {
		t.Fatalf("page 2 order not preserved: %#v", second.Items)
	}
	got := []string{first.Items[0].LUID, first.Items[1].LUID, second.Items[0].LUID, second.Items[1].LUID}
	want := []string{"1", "2", "3", "4"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("global order = %v, want %v", got, want)
		}
	}
	if len(src.cursors) != 2 || src.cursors[1] != "upstream-next" {
		t.Fatalf("upstream cursors = %#v", src.cursors)
	}
}

func assertGolden(t *testing.T, path string, value any, full bool) {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
