package search

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestListSourcesPreservesFiltersWithSourceBoundedContinuation(t *testing.T) {
	calls := 0
	sources := ListSources{"workbook": func(_ context.Context, input value.SearchRequest) (value.SearchPage, error) {
		calls++
		if input.Cursor != "inner" || input.Limit != 2 || input.ProjectPath != "Parent/Child" || input.Owner != "owner" {
			t.Fatalf("input=%+v", input)
		}
		return value.SearchPage{NextCursor: "next"}, nil
	}}
	page, err := sources.SearchPage(t.Context(), "workbook", "inner", 2, value.SearchRequest{Cursor: "outer", Limit: 20, ProjectPath: "Parent/Child", Owner: "owner"})
	if err != nil || page.NextCursor != "next" || calls != 1 {
		t.Fatalf("page=%+v err=%v calls=%d", page, err, calls)
	}
	if _, err := sources.SearchPage(t.Context(), "missing", "", 2, value.SearchRequest{}); err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
