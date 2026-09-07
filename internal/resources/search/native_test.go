package search_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
)

type nativeClient struct {
	requests []tableausearch.Request
	pages    map[int]tableausearch.Page
}

type datasourceResolver struct {
	calls   [][]string
	results map[string]string
	err     error
}

func (r *datasourceResolver) ResolveContentURLs(_ context.Context, values []string) (map[string]string, error) {
	r.calls = append(r.calls, append([]string(nil), values...))
	return r.results, r.err
}

func (c *nativeClient) Search(_ context.Context, input tableausearch.Request) (tableausearch.Page, error) {
	c.requests = append(c.requests, input)
	return c.pages[input.Page], nil
}

func TestNativeAdapterProjectsContentAndContinuesWithBoundedCursor(t *testing.T) {
	client := &nativeClient{pages: map[int]tableausearch.Page{
		0: {Items: []tableausearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales", ProjectPath: "Department/Sales", OwnerName: "Analyst", ModifiedAt: "2026-09-05"}}, PageIndex: 0, StartIndex: 0, Limit: 1, Total: 2, HasNext: true, TableauRequestID: "request-1"},
		1: {Items: []tableausearch.Item{{LUID: "native-ds-1", Type: "datasource", Name: "Sales Source", ContentURL: "sales-source"}}, PageIndex: 1, StartIndex: 1, Limit: 1, Total: 2, TableauRequestID: "request-2"},
	}}
	resolver := &datasourceResolver{results: map[string]string{"sales-source": "ds-1"}}
	adapter := resourcesearch.NewNativeAdapter(client, resolver)
	input := resourcesearch.Input{Types: []string{"workbook", "datasource"}, Terms: "sales", Limit: 1}
	first, err := adapter.Search(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got := first.Items; !reflect.DeepEqual(got, []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales", ProjectPath: "Department/Sales", Owner: "Analyst", ModifiedAt: "2026-09-05"}}) || first.NextCursor == "" {
		t.Fatalf("first=%+v", first)
	}
	if first.TableauRequestID != "request-1" {
		t.Fatalf("request ID=%q", first.TableauRequestID)
	}
	if first.Total != 2 {
		t.Fatalf("total=%d", first.Total)
	}
	input.Cursor = first.NextCursor
	second, err := adapter.Search(context.Background(), input)
	if err != nil || second.NextCursor != "" || len(second.Items) != 1 || second.Items[0].LUID != "ds-1" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if got := []int{client.requests[0].Page, client.requests[1].Page}; !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("pages=%v", got)
	}
	if !reflect.DeepEqual(resolver.calls, [][]string{{"sales-source"}}) {
		t.Fatalf("resolver calls=%v", resolver.calls)
	}
}

func TestNativeAdapterRejectsChangedOrOversizedCursor(t *testing.T) {
	client := &nativeClient{pages: map[int]tableausearch.Page{0: {Items: []tableausearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}, PageIndex: 0, Limit: 1, Total: 2, HasNext: true}}}
	adapter := resourcesearch.NewNativeAdapter(client, nil)
	input := resourcesearch.Input{Types: []string{"workbook"}, Terms: "sales", Limit: 1}
	first, err := adapter.Search(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Cursor = first.NextCursor
	input.Terms = "changed"
	_, err = adapter.Search(context.Background(), input)
	var invalid interface{ InvalidSearchCursor() bool }
	if !errors.As(err, &invalid) {
		t.Fatalf("changed cursor error=%v", err)
	}
	input.Cursor = string(make([]byte, 4097))
	_, err = adapter.Search(context.Background(), input)
	if !errors.As(err, &invalid) {
		t.Fatalf("oversized cursor error=%v", err)
	}
}

func TestNativeAdapterRejectsUnsupportedClientSideFilters(t *testing.T) {
	adapter := resourcesearch.NewNativeAdapter(&nativeClient{pages: map[int]tableausearch.Page{}}, nil)
	for _, input := range []resourcesearch.Input{
		{Types: []string{"workbook"}, Limit: 10, ProjectPath: "Department/Sales"},
		{Types: []string{"workbook"}, Limit: 10, Owner: "Analyst"},
	} {
		if _, err := adapter.Search(context.Background(), input); err == nil {
			t.Fatalf("accepted %#v", input)
		}
	}
}

func TestNativeAdapterRejectsResponseDrift(t *testing.T) {
	for _, page := range []tableausearch.Page{
		{PageIndex: 1, Limit: 10},
		{PageIndex: 0, Limit: 10, Items: []tableausearch.Item{{LUID: "wb-1", Type: "user", Name: "Analyst"}}},
		{PageIndex: 0, Limit: 10, Items: []tableausearch.Item{{LUID: "", Type: "workbook", Name: "Sales"}}},
	} {
		adapter := resourcesearch.NewNativeAdapter(&nativeClient{pages: map[int]tableausearch.Page{0: page}}, nil)
		if _, err := adapter.Search(context.Background(), resourcesearch.Input{Types: []string{"workbook"}, Limit: 10}); err == nil {
			t.Fatalf("accepted %+v", page)
		}
	}
}

func TestNativeAdapterRequiresAuthoritativeDatasourceResolution(t *testing.T) {
	page := tableausearch.Page{PageIndex: 0, Limit: 10, Total: 1, Items: []tableausearch.Item{{LUID: "native-ds", Type: "datasource", Name: "Sales", ContentURL: "sales"}}}
	client := &nativeClient{pages: map[int]tableausearch.Page{0: page}}
	if _, err := resourcesearch.NewNativeAdapter(client, nil).Search(context.Background(), resourcesearch.Input{Types: []string{"datasource"}, Terms: "sales", Limit: 10}); err == nil {
		t.Fatal("accepted a datasource result without a configured resolver")
	}
	for _, resolver := range []*datasourceResolver{
		{results: map[string]string{}},
		{err: errors.New("resolution failed")},
	} {
		adapter := resourcesearch.NewNativeAdapter(client, resolver)
		if _, err := adapter.Search(context.Background(), resourcesearch.Input{Types: []string{"datasource"}, Terms: "sales", Limit: 10}); err == nil {
			t.Fatal("accepted a datasource result without an authoritative classic REST LUID")
		}
	}
}

func TestNativeAdapterDoesNotResolvePagesWithoutDatasources(t *testing.T) {
	page := tableausearch.Page{PageIndex: 0, Limit: 10, Total: 1, Items: []tableausearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}
	resolver := &datasourceResolver{results: map[string]string{"unused": "ds-1"}}
	adapter := resourcesearch.NewNativeAdapter(&nativeClient{pages: map[int]tableausearch.Page{0: page}}, resolver)
	if _, err := adapter.Search(context.Background(), resourcesearch.Input{Types: []string{"workbook"}, Terms: "sales", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("resolver calls=%v", resolver.calls)
	}
}
