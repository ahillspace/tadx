package search

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type completeListPagerCall struct {
	resourceType string
	cursor       string
	limit        int
	input        value.SearchRequest
}

type completeListPagerFake struct {
	pages []value.SearchPage
	calls []completeListPagerCall
}

func (f *completeListPagerFake) SearchPage(_ context.Context, resourceType, cursor string, limit int, input value.SearchRequest) (value.SearchPage, error) {
	f.calls = append(f.calls, completeListPagerCall{resourceType: resourceType, cursor: cursor, limit: limit, input: input})
	if len(f.pages) == 0 {
		return value.SearchPage{}, errors.New("unexpected complete list page call")
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

func TestCompleteListSearchFamilyContinuationPreservesChildLimitAndPhase(t *testing.T) {
	pager := &completeListPagerFake{pages: []value.SearchPage{
		{Items: []value.SearchItem{{LUID: "datasource-1", Type: "datasource", Name: "Datasource"}}, Source: "live"},
		{Items: []value.SearchItem{{LUID: "flow-1", Type: "flow", Name: "Flow A"}}, NextCursor: "flow-next", Source: "live"},
		{Items: []value.SearchItem{{LUID: "flow-2", Type: "flow", Name: "Flow B"}}, Source: "cache"},
		{Items: []value.SearchItem{{LUID: "project-1", Type: "project", Name: "Project"}}, Source: "live"},
	}}
	adapter := &CompleteLists{Lister: pager}
	input := value.SearchRequest{Types: []string{"datasource", "flow", "project", "workbook"}, ProjectPath: "Sales/Ops", Owner: "owner-1", Limit: 2}
	first, err := adapter.Search(t.Context(), input)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" || first.Source != "live" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	input.Cursor = first.NextCursor
	second, err := adapter.Search(t.Context(), input)
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

func TestSingleSourceListSearchPropagatesTotal(t *testing.T) {
	adapter := &CompleteLists{Lister: &completeListPagerFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "wb-1", Type: "workbook", Name: "One"}}, Total: 42, NextCursor: "more"}}}}
	out, err := executeSearchAction(t.Context(), LiveSource{Lists: adapter}, Input{Environment: "dev", SiteResolved: true, Type: "workbook", Limit: 1})
	if err != nil || out.Page.Total != 42 {
		t.Fatalf("list search = %#v, %v", out, err)
	}
}

func TestGroupedSearchRetainsUnresolvedTruncationAcrossActionPages(t *testing.T) {
	for _, unresolved := range []bool{false, true} {
		pages := []value.SearchPage{
			{Items: []value.SearchItem{{LUID: "g", Type: "group", Name: "Group"}}, MoreAvailable: unresolved},
			{Items: []value.SearchItem{{LUID: "u1", Type: "user", Name: "User1"}}, NextCursor: "users-next", MoreAvailable: true},
			{Items: []value.SearchItem{{LUID: "u2", Type: "user", Name: "User2"}}},
		}
		adapter := &CompleteLists{Lister: &completeListPagerFake{pages: pages}}
		out, err := executeSearchAction(t.Context(), LiveSource{Lists: adapter}, Input{Environment: "dev", SiteResolved: true, Type: "admin", Limit: 200})
		if err != nil || out.Page.MoreAvailable != unresolved || len(out.Items) != 3 {
			t.Fatalf("unresolved=%t out=%+v err=%v", unresolved, out, err)
		}
	}
}
