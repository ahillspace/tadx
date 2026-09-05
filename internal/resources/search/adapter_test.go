package search_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/resources/search"
)

type lister struct {
	pages map[string]search.Page
	calls []string
}

func (s *lister) List(_ context.Context, kind, cursor string, limit int) (search.Page, error) {
	s.calls = append(s.calls, kind+":"+cursor)
	return s.pages[kind+":"+cursor], nil
}

func TestAdapterFiltersDeterministicallyAndContinuesWithinPage(t *testing.T) {
	s := &lister{pages: map[string]search.Page{
		"workbook:":     {Items: []search.Item{{LUID: "3", Type: "workbook", Name: "Sales C"}, {LUID: "1", Type: "workbook", Name: "Sales A"}, {LUID: "2", Type: "workbook", Name: "Other"}, {LUID: "4", Type: "workbook", Name: "Sales B"}}, NextCursor: "next"},
		"workbook:next": {Items: []search.Item{{LUID: "5", Type: "workbook", Name: "Sales D"}}},
	}}
	a := search.NewAdapter(s)
	in := search.Input{Types: []string{"workbook"}, Terms: "sales", Limit: 2}
	first, err := a.Search(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{first.Items[0].LUID, first.Items[1].LUID}; !reflect.DeepEqual(got, []string{"1", "4"}) || first.NextCursor == "" {
		t.Fatalf("page=%+v", first)
	}
	in.Cursor = first.NextCursor
	second, err := a.Search(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{second.Items[0].LUID, second.Items[1].LUID}; !reflect.DeepEqual(got, []string{"3", "5"}) || second.NextCursor != "" {
		t.Fatalf("page=%+v", second)
	}
}

func TestAdapterRejectsPageDriftAndChangedFilters(t *testing.T) {
	s := &lister{pages: map[string]search.Page{"user:": {Items: []search.Item{{LUID: "1", Type: "user", Name: "A"}, {LUID: "2", Type: "user", Name: "B"}}}}}
	in := search.Input{Types: []string{"user"}, Limit: 1}
	a := search.NewAdapter(s)
	first, err := a.Search(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	in.Cursor = first.NextCursor
	in.Terms = "changed"
	_, err = a.Search(context.Background(), in)
	var invalid interface{ InvalidSearchCursor() bool }
	if !errors.As(err, &invalid) {
		t.Fatalf("filter error=%v", err)
	}
	in.Terms = ""
	s.pages["user:"] = search.Page{Items: []search.Item{{LUID: "3", Type: "user", Name: "C"}, {LUID: "2", Type: "user", Name: "B"}}}
	_, err = a.Search(context.Background(), in)
	if !errors.As(err, &invalid) {
		t.Fatalf("drift error=%v", err)
	}
}

type endlessLister struct{ calls int }

func (s *endlessLister) List(_ context.Context, kind, cursor string, limit int) (search.Page, error) {
	s.calls++
	return search.Page{NextCursor: fmt.Sprint(s.calls), Items: []search.Item{{LUID: fmt.Sprint(s.calls), Type: kind, Name: "Other"}}}, nil
}
func TestAdapterBoundsScanningAndReturnsContinuation(t *testing.T) {
	s := &endlessLister{}
	page, err := search.NewAdapter(s).Search(context.Background(), search.Input{Types: []string{"workbook"}, Terms: "absent", Limit: 2})
	if err != nil || s.calls != 100 || len(page.Items) != 0 || page.NextCursor == "" || len(page.Warnings) == 0 {
		t.Fatalf("calls=%d page=%+v error=%v", s.calls, page, err)
	}
}

func TestAdapterRejectsInvalidIdentityAndRepeatedCursor(t *testing.T) {
	for _, page := range []search.Page{{Items: []search.Item{{Type: "user", Name: "A"}}}, {Items: []search.Item{{LUID: "1", Type: "group", Name: "A"}}}, {Items: []search.Item{{LUID: "1", Type: "user", Name: "A"}, {LUID: "1", Type: "user", Name: "B"}}}} {
		s := &lister{pages: map[string]search.Page{"user:": page}}
		if _, err := search.NewAdapter(s).Search(context.Background(), search.Input{Types: []string{"user"}, Limit: 20}); err == nil {
			t.Fatalf("accepted %+v", page)
		}
	}
	s := &lister{pages: map[string]search.Page{"user:": {NextCursor: "loop"}, "user:loop": {NextCursor: "loop"}}}
	if _, err := search.NewAdapter(s).Search(context.Background(), search.Input{Types: []string{"user"}, Limit: 20}); err == nil {
		t.Fatal("accepted a repeating source cursor")
	}
}

func TestAdapterOrdersTypesAndAppliesExactScopeFilters(t *testing.T) {
	s := &lister{pages: map[string]search.Page{
		"workbook:":   {Items: []search.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales", ProjectPath: "Ops", Owner: "owner-1"}}},
		"datasource:": {Items: []search.Item{{LUID: "ds-1", Type: "datasource", Name: "Sales", ProjectPath: "Ops", Owner: "owner-1"}, {LUID: "ds-2", Type: "datasource", Name: "Sales", ProjectPath: "Other", Owner: "owner-1"}, {LUID: "ds-3", Type: "datasource", Name: "Sales", ProjectPath: "Ops", Owner: "owner-2"}}},
	}}
	page, err := search.NewAdapter(s).Search(context.Background(), search.Input{Types: []string{"workbook", "datasource"}, ProjectPath: "Ops", Owner: "owner-1", Terms: "sales", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{page.Items[0].LUID, page.Items[1].LUID}; !reflect.DeepEqual(got, []string{"ds-1", "wb-1"}) {
		t.Fatalf("items=%+v", page.Items)
	}
}

func TestAdapterSearchBoundedUsesPartialBudgetWithoutBindingCursorToIt(t *testing.T) {
	s := &lister{pages: map[string]search.Page{"user:": {Items: []search.Item{
		{LUID: "1", Type: "user", Name: "A"},
		{LUID: "2", Type: "user", Name: "B"},
		{LUID: "3", Type: "user", Name: "C"},
	}}}}
	adapter := search.NewAdapter(s)
	input := search.Input{Types: []string{"user"}, Limit: 3}
	first, err := adapter.SearchBounded(context.Background(), input, 1)
	if err != nil || len(first.Items) != 1 || first.Items[0].LUID != "1" || first.NextCursor == "" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	input.Cursor = first.NextCursor
	second, err := adapter.Search(context.Background(), input)
	if err != nil || len(second.Items) != 2 || second.Items[0].LUID != "2" || second.Items[1].LUID != "3" || second.NextCursor != "" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
}
