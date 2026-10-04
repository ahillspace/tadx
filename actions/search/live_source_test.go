package search

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/value"
	"reflect"
	"testing"
)

func executeSearchAction(ctx context.Context, source Source, input Input) (Output, error) {
	types, err := ValidateInput(input)
	if err != nil {
		return Output{}, err
	}
	return Execute(ctx, source, input, types)
}

func searchWithTypes(ctx context.Context, source LiveSource, input Input) (Result, error) {
	types, err := ValidateInput(input)
	if err != nil {
		return Result{}, err
	}
	return source.Search(ctx, input, types)
}

type appSearchFake struct {
	pages   []value.SearchPage
	inputs  []value.SearchRequest
	budgets []int
}

func (s *appSearchFake) Search(_ context.Context, input value.SearchRequest) (value.SearchPage, error) {
	s.inputs = append(s.inputs, input)
	if len(s.pages) == 0 {
		return value.SearchPage{}, errors.New("unexpected search call")
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func (s *appSearchFake) SearchBounded(_ context.Context, input value.SearchRequest, budget int) (value.SearchPage, error) {
	s.inputs = append(s.inputs, input)
	s.budgets = append(s.budgets, budget)
	if len(s.pages) == 0 {
		return value.SearchPage{}, errors.New("unexpected bounded search call")
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func TestLiveGlobalSearchRoutesNonemptyContentTermsToNativeSearch(t *testing.T) {
	for _, test := range []struct {
		selector string
		types    []string
	}{{"content", []string{"datasource", "flow", "project", "workbook"}}, {"workbook", []string{"workbook"}}} {
		native := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
		dedicated := &appSearchFake{}
		source := LiveSource{Native: native, Dedicated: dedicated}
		result, err := searchWithTypes(t.Context(), source, Input{Terms: "sales", Type: test.selector, Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) != 1 || len(native.inputs) != 1 || len(dedicated.inputs) != 0 {
			t.Fatalf("selector=%s result=%+v native=%+v dedicated=%+v", test.selector, result, native.inputs, dedicated.inputs)
		}
		if !reflect.DeepEqual(native.inputs[0].Types, test.types) || native.inputs[0].Terms != "sales" || native.inputs[0].Limit != 20 {
			t.Fatalf("selector=%s native input=%+v", test.selector, native.inputs[0])
		}
	}
}

func TestLiveGlobalSearchRetainsDedicatedRoutesForAdminAndPulse(t *testing.T) {
	for _, selector := range []string{"admin", "user", "group", "pulse", "definition", "metric"} {
		native := &appSearchFake{}
		dedicated := &appSearchFake{pages: []value.SearchPage{{}}}
		source := LiveSource{Native: native, Dedicated: dedicated}
		if _, err := searchWithTypes(t.Context(), source, Input{Terms: "sales", Type: selector, Limit: 20}); err != nil {
			t.Fatalf("selector=%s error=%v", selector, err)
		}
		if len(native.inputs) != 0 || len(dedicated.inputs) != 1 {
			t.Fatalf("selector=%s native=%d dedicated=%d", selector, len(native.inputs), len(dedicated.inputs))
		}
	}
}

func TestLiveGlobalSearchRetainsListSemanticsForBlankTypedSearch(t *testing.T) {
	for _, selector := range []string{"workbook", "datasource", "flow", "project", "content", "user", "group", "admin"} {
		native := &appSearchFake{}
		lists := &appSearchFake{pages: []value.SearchPage{{}}}
		dedicated := &appSearchFake{}
		source := LiveSource{Native: native, Lists: lists, Dedicated: dedicated}
		result, err := searchWithTypes(t.Context(), source, Input{Type: selector, Limit: 20})
		if err != nil || len(result.Items) != 0 || len(native.inputs) != 0 || len(lists.inputs) != 1 || len(dedicated.inputs) != 0 {
			t.Fatalf("selector=%s result=%+v error=%v native=%d lists=%d dedicated=%d", selector, result, err, len(native.inputs), len(lists.inputs), len(dedicated.inputs))
		}
	}
}

func TestLiveGlobalSearchRetainsDedicatedListSemanticsForBlankPulseSearch(t *testing.T) {
	for _, selector := range []string{"definition", "metric", "pulse"} {
		native := &appSearchFake{}
		lists := &appSearchFake{}
		dedicated := &appSearchFake{pages: []value.SearchPage{{}}}
		source := LiveSource{Native: native, Lists: lists, Dedicated: dedicated}
		if _, err := searchWithTypes(t.Context(), source, Input{Type: selector, Limit: 20}); err != nil {
			t.Fatalf("selector=%s error=%v", selector, err)
		}
		if len(native.inputs) != 0 || len(lists.inputs) != 0 || len(dedicated.inputs) != 1 {
			t.Fatalf("selector=%s native=%d lists=%d dedicated=%d", selector, len(native.inputs), len(lists.inputs), len(dedicated.inputs))
		}
	}
}

func TestUntypedLiveSearchReturnsNativeContentBeforeDedicatedResults(t *testing.T) {
	native := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	dedicated := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "user-1", Type: "user", Name: "Sales Analyst"}}, NextCursor: "dedicated-page-2"}, {Items: []value.SearchItem{{LUID: "user-2", Type: "user", Name: "Sales Manager"}}}}}
	source := LiveSource{Native: native, Dedicated: dedicated}
	input := Input{Terms: "sales", Limit: 2}

	first, err := searchWithTypes(t.Context(), source, input)
	if err != nil || len(first.Items) != 2 || first.Items[0].Type != "workbook" || first.Items[1].Type != "user" || first.Page.NextCursor == "" || len(dedicated.inputs) != 1 || !reflect.DeepEqual(dedicated.budgets, []int{1}) {
		t.Fatalf("first=%+v error=%v dedicated=%d budgets=%v", first, err, len(dedicated.inputs), dedicated.budgets)
	}
	input.Cursor = first.Page.NextCursor
	second, err := searchWithTypes(t.Context(), source, input)
	if err != nil || len(second.Items) != 1 || second.Items[0].LUID != "user-2" || second.Page.NextCursor != "" || len(native.inputs) != 1 || len(dedicated.inputs) != 2 {
		t.Fatalf("second=%+v error=%v native=%d dedicated=%d", second, err, len(native.inputs), len(dedicated.inputs))
	}
	wantTypes := []string{"definition", "group", "metric", "user"}
	if !reflect.DeepEqual(dedicated.inputs[0].Types, wantTypes) || dedicated.inputs[0].Cursor != "" {
		t.Fatalf("dedicated input=%+v", dedicated.inputs[0])
	}
}

func TestUntypedLiveSearchExactNativeLimitReturnsDedicatedPhaseCursor(t *testing.T) {
	native := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	dedicated := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "group-1", Type: "group", Name: "Sales"}}}}}
	source := LiveSource{Native: native, Dedicated: dedicated}
	input := Input{Terms: "sales", Limit: 1}
	first, err := searchWithTypes(t.Context(), source, input)
	if err != nil || len(first.Items) != 1 || first.Page.NextCursor == "" || len(dedicated.inputs) != 0 {
		t.Fatalf("first=%+v error=%v dedicated=%d", first, err, len(dedicated.inputs))
	}
	input.Cursor = first.Page.NextCursor
	second, err := searchWithTypes(t.Context(), source, input)
	if err != nil || len(second.Items) != 1 || second.Items[0].Type != "group" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
}

func TestUntypedLiveSearchWithNoNativeMatchesReturnsDedicatedPageImmediately(t *testing.T) {
	native := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{}}}}
	dedicated := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "user-1", Type: "user", Name: "Sales"}}}}}
	source := LiveSource{Native: native, Dedicated: dedicated}
	result, err := searchWithTypes(t.Context(), source, Input{Terms: "sales", Limit: 20})
	if err != nil || len(result.Items) != 1 || result.Items[0].Type != "user" || len(native.inputs) != 1 || len(dedicated.inputs) != 1 || len(dedicated.budgets) != 0 {
		t.Fatalf("result=%+v error=%v native=%d dedicated=%d budgets=%v", result, err, len(native.inputs), len(dedicated.inputs), dedicated.budgets)
	}
}

func TestUntypedLiveSearchPreservesNativeContinuationBeforePhaseChange(t *testing.T) {
	native := &appSearchFake{pages: []value.SearchPage{
		{Items: []value.SearchItem{{LUID: "wb-1", Type: "workbook", Name: "Sales A"}}, NextCursor: "native-page-2"},
		{Items: []value.SearchItem{{LUID: "wb-2", Type: "workbook", Name: "Sales B"}}},
	}}
	dedicated := &appSearchFake{}
	source := LiveSource{Native: native, Dedicated: dedicated}
	input := Input{Terms: "sales", Limit: 1}

	first, err := searchWithTypes(t.Context(), source, input)
	if err != nil || first.Page.NextCursor == "" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	input.Cursor = first.Page.NextCursor
	second, err := searchWithTypes(t.Context(), source, input)
	if err != nil || second.Items[0].LUID != "wb-2" || second.Page.NextCursor == "" || native.inputs[1].Cursor != "native-page-2" || len(dedicated.inputs) != 0 {
		t.Fatalf("second=%+v error=%v native=%+v", second, err, native.inputs)
	}
}

func TestUntypedLiveSearchRejectsChangedCompositeCursor(t *testing.T) {
	native := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	source := LiveSource{Native: native, Dedicated: &appSearchFake{}}
	input := Input{Terms: "sales", Limit: 1}
	first, err := searchWithTypes(t.Context(), source, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Cursor = first.Page.NextCursor
	input.Terms = "changed"
	_, err = searchWithTypes(t.Context(), source, input)
	if _, ok := errors.AsType[interface {
		error
		InvalidSearchCursor() bool
	}](err); !ok {
		t.Fatalf("error=%v", err)
	}
}

func TestUntypedLiveSearchContinuationRoundTripsThroughActionCursor(t *testing.T) {
	native := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
	dedicated := &appSearchFake{pages: []value.SearchPage{{Items: []value.SearchItem{{LUID: "group-1", Type: "group", Name: "Sales Team"}}}}}
	source := LiveSource{Native: native, Dedicated: dedicated}
	input := Input{Terms: "sales", Environment: "dev", Site: "site", SiteResolved: true, Limit: 1}

	first, err := executeSearchAction(t.Context(), source, input)
	if err != nil || first.Page.NextCursor == "" || first.Items[0].Type != "workbook" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	input.Cursor = first.Page.NextCursor
	second, err := executeSearchAction(t.Context(), source, input)
	if err != nil || second.Page.NextCursor != "" || second.Items[0].Type != "group" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
}

func TestUntypedLiveSearchRejectsOversizedNestedContinuation(t *testing.T) {
	native := &appSearchFake{pages: []value.SearchPage{{NextCursor: string(make([]byte, maxCombinedSourceBytes+1))}}}
	source := LiveSource{Native: native, Dedicated: &appSearchFake{}}
	if _, err := searchWithTypes(t.Context(), source, Input{Terms: "sales", Limit: 20}); err == nil {
		t.Fatal("accepted an oversized nested continuation")
	}
}
