package search_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
	"github.com/ahillspace/tadx/internal/value"
)

func TestNativeAdapterOutputBudgetPreservesPageSizeAndRankedContinuation(t *testing.T) {
	for _, freshCommand := range []bool{false, true} {
		t.Run(fmt.Sprintf("fresh_command_%t", freshCommand), func(t *testing.T) {
			const total = 135
			client := &nativeClient{pages: make(map[int]tableausearch.Page)}
			for index := range 2 {
				page := tableausearch.Page{PageIndex: index, Limit: 100, Total: total, HasNext: index == 0}
				for position := index * 100; position < min((index+1)*100, total); position++ {
					page.Items = append(page.Items, tableausearch.Item{LUID: fmt.Sprintf("wb-%03d", position), Type: "workbook", Name: fmt.Sprintf("Workbook %03d", position)})
				}
				client.pages[index] = page
			}
			adapter := resourcesearch.NewNativeAdapter(client, nil)
			input := resourcesearch.Input{Types: []string{"workbook"}, Terms: "test", Limit: 100}
			position := 0
			for _, budget := range []int{1, 30, 30, 30, 30, 30} {
				if freshCommand {
					adapter = resourcesearch.NewNativeAdapter(client, nil)
				}
				page, err := adapter.SearchBounded(t.Context(), input, budget)
				if err != nil {
					t.Fatal(err)
				}
				wantCount := min(budget, total-position)
				if len(page.Items) != wantCount {
					t.Fatalf("offset=%d budget=%d returned %d rows, want %d", position, budget, len(page.Items), wantCount)
				}
				for offset, item := range page.Items {
					want := fmt.Sprintf("wb-%03d", position+offset)
					if item.LUID != want || item.Type != "workbook" {
						t.Fatalf("rank %d: got %+v, want workbook %q", position+offset, item, want)
					}
				}
				position += wantCount
				wantMore := position < total
				if page.Total != total || page.MoreAvailable != wantMore || (page.NextCursor != "") != wantMore || page.UnresolvedMoreAvailable {
					t.Fatalf("offset=%d total=%d more=%t cursor=%q unresolved=%t", position, page.Total, page.MoreAvailable, page.NextCursor, page.UnresolvedMoreAvailable)
				}
				input.Cursor = page.NextCursor
			}
			if position != total {
				t.Fatalf("consumed %d rows, want %d", position, total)
			}
			var pages []int
			for _, request := range client.requests {
				if request.Limit != 100 || request.Terms != "test" {
					t.Fatalf("budget changed native request: %+v", request)
				}
				pages = append(pages, request.Page)
			}
			wantPages := []int{0, 1}
			if freshCommand {
				wantPages = []int{0, 0, 0, 0, 0, 1, 0, 1}
			}
			if !reflect.DeepEqual(pages, wantPages) {
				t.Fatalf("native pages=%v, want %v", pages, wantPages)
			}
		})
	}
}

func TestNativeMetricBudgetDefersNextPageAndPreservesContinuation(t *testing.T) {
	for _, freshCommand := range []bool{false, true} {
		t.Run(fmt.Sprintf("fresh_command_%t", freshCommand), func(t *testing.T) {
			firstHit := tableausearch.Item{LUID: "metric-1", Type: "metric", Name: "First"}
			client := &nativeClient{pages: map[int]tableausearch.Page{
				0: {PageIndex: 0, Limit: 2, Total: 4, HasNext: true, Items: []tableausearch.Item{firstHit, {LUID: "metric-2", Type: "metric", Name: "Second"}}},
			}}
			adapter := resourcesearch.NewNativeAdapter(client, nil)
			input := resourcesearch.Input{Types: []string{"metric"}, Terms: "test", Limit: 2}
			first, err := adapter.Search(t.Context(), input)
			if err != nil || len(first.Items) != 2 || first.Items[0].LUID != "metric-1" || first.Items[1].LUID != "metric-2" || len(client.requests) != 1 || !first.MoreAvailable || first.NextCursor == "" {
				t.Fatalf("first=%+v err=%v requests=%v", first, err, client.requests)
			}
			// Only a continuation needs this page. Its repeated identity must
			// still be detected, with the original rank retained exactly once.
			client.pages[1] = tableausearch.Page{PageIndex: 1, Limit: 2, Total: 4, Items: []tableausearch.Item{firstHit, {LUID: "metric-3", Type: "metric", Name: "Third"}}}
			if freshCommand {
				adapter = resourcesearch.NewNativeAdapter(client, nil)
			}
			input.Cursor = first.NextCursor
			last, err := adapter.Search(t.Context(), input)
			if err != nil || len(last.Items) != 1 || last.Items[0].LUID != "metric-3" || !last.UnresolvedMoreAvailable || !last.MoreAvailable || len(last.Warnings) != 1 || last.NextCursor != "" || last.Total != 0 {
				t.Fatalf("continuation=%+v err=%v", last, err)
			}
			// A fresh command must validate the prefix before accepting this
			// page-boundary cursor, rather than trusting the deferred offset.
			page := client.pages[0]
			page.Items = append([]tableausearch.Item(nil), page.Items...)
			page.Items[0].Name = "Changed"
			client.pages[0] = page
			_, err = resourcesearch.NewNativeAdapter(client, nil).Search(t.Context(), input)
			invalid, ok := errors.AsType[interface {
				error
				InvalidSearchCursor() bool
			}](err)
			if !ok || !invalid.InvalidSearchCursor() {
				t.Fatalf("changed metric prefix should reject a deferred-page cursor: %v", err)
			}
		})
	}
}

func TestNativeAdapterOutputBudgetCountsCanonicalContent(t *testing.T) {
	client := &nativeClient{pages: map[int]tableausearch.Page{
		0: {PageIndex: 0, Limit: 100, Total: 4, Items: []tableausearch.Item{
			{LUID: "wb-1", Type: "workbook", Name: "First"},
			publishedHit("connection-a", "ds-1", "sales"),
			publishedHit("connection-b", "ds-1", "sales"),
			{LUID: "wb-2", Type: "workbook", Name: "Last"},
		}},
	}}
	resolver := &datasourceResolver{results: map[string]value.Datasource{"sales": {LUID: "ds-1"}}}
	adapter := resourcesearch.NewNativeAdapter(client, resolver)
	input := resourcesearch.Input{Types: []string{"workbook", "datasource"}, Terms: "sales", Limit: 100}
	for index, want := range []resourcesearch.Item{
		{LUID: "wb-1", Type: "workbook", Name: "First"},
		{LUID: "ds-1", Type: "datasource", Name: "Sales"},
		{LUID: "wb-2", Type: "workbook", Name: "Last"},
	} {
		page, err := adapter.SearchBounded(t.Context(), input, 1)
		if err != nil {
			t.Fatal(err)
		}
		wantTotal := 3
		if index < 2 {
			wantTotal = 0 // The remaining native connections have not been scanned.
		}
		if index == 0 && len(resolver.calls) != 0 {
			t.Fatalf("the first workbook required unused datasource resolution: %v", resolver.calls)
		}
		wantMore := index < 2
		if !reflect.DeepEqual(page.Items, []resourcesearch.Item{want}) || page.Total != wantTotal || page.MoreAvailable != wantMore || (page.NextCursor != "") != wantMore || page.UnresolvedMoreAvailable {
			t.Fatalf("page %d=%+v, want %+v total=%d more=%t", index, page, want, wantTotal, wantMore)
		}
		input.Cursor = page.NextCursor
	}
	if len(client.requests) != 1 || !reflect.DeepEqual(resolver.calls, [][]string{{"sales"}}) {
		t.Fatalf("native requests=%v resolver calls=%v", client.requests, resolver.calls)
	}
}

func TestNativeAdapterBatchesOnlyRequiredDatasourcePrefixes(t *testing.T) {
	for _, freshCommand := range []bool{false, true} {
		t.Run(fmt.Sprintf("fresh_command_%t", freshCommand), func(t *testing.T) {
			page := tableausearch.Page{PageIndex: 0, Limit: 100, Total: 40}
			resolver := &datasourceResolver{results: make(map[string]value.Datasource)}
			urls := make([]string, 40)
			for index := range urls {
				urls[index] = fmt.Sprintf("source-%02d", index)
				id := fmt.Sprintf("ds-%02d", index)
				page.Items = append(page.Items, publishedHit(fmt.Sprintf("connection-%02d", index), id, urls[index]))
				resolver.results[urls[index]] = value.Datasource{LUID: id}
			}
			client := &nativeClient{pages: map[int]tableausearch.Page{0: page}}
			adapter := resourcesearch.NewNativeAdapter(client, resolver)
			input := resourcesearch.Input{Types: []string{"datasource"}, Limit: 100}
			first, err := adapter.SearchBounded(t.Context(), input, 30)
			if err != nil || len(first.Items) != 30 || first.Items[29].LUID != "ds-29" || first.NextCursor == "" || first.Total != 0 || !reflect.DeepEqual(resolver.calls, [][]string{urls[:30]}) {
				t.Fatalf("first=%+v err=%v resolver calls=%v", first, err, resolver.calls)
			}
			if freshCommand {
				adapter = resourcesearch.NewNativeAdapter(client, resolver)
			}
			input.Cursor = first.NextCursor
			next, err := adapter.SearchBounded(t.Context(), input, 1)
			wantCalls := [][]string{urls[:30], urls[30:31]}
			if freshCommand {
				wantCalls = [][]string{urls[:30], urls[:30], urls[30:31]}
			}
			if err != nil || len(next.Items) != 1 || next.Items[0].LUID != "ds-30" || next.NextCursor == "" || !reflect.DeepEqual(resolver.calls, wantCalls) {
				t.Fatalf("continuation=%+v err=%v resolver calls=%v", next, err, resolver.calls)
			}
		})
	}
}

func TestNativeAdapterRejectsInvalidOutputBudgetBeforeIO(t *testing.T) {
	for _, test := range []struct{ limit, budget int }{{100, -1}, {100, 0}, {100, 101}, {10, 11}} {
		t.Run(fmt.Sprintf("limit_%d_budget_%d", test.limit, test.budget), func(t *testing.T) {
			client := &nativeClient{}
			_, err := resourcesearch.NewNativeAdapter(client, nil).SearchBounded(t.Context(), resourcesearch.Input{Types: []string{"workbook"}, Limit: test.limit}, test.budget)
			if err == nil || len(client.requests) != 0 {
				t.Fatalf("err=%v requests=%v", err, client.requests)
			}
		})
	}
}
