package search_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
)

func publishedHit(hit, parent, url string) tableausearch.Item {
	return tableausearch.Item{LUID: hit, Type: "datasource", Name: "Connection label", ContentURL: url,
		DatasourceLUID: parent, DatasourceIsPublished: new(true), ParentType: "Datasource", ParentLUID: parent, ParentName: "Sales"}
}

func TestNativeAdapterNormalizesRankedConnectionsAcrossPages(t *testing.T) {
	client := &nativeClient{pages: map[int]tableausearch.Page{
		0: {PageIndex: 0, Limit: 2, Total: 5, HasNext: true, Items: []tableausearch.Item{
			{LUID: "wb-1", Type: "workbook", Name: "First"}, publishedHit("connection-a", "ds-1", "sales")}},
		1: {PageIndex: 1, Limit: 2, Total: 5, HasNext: true, Items: []tableausearch.Item{
			publishedHit("connection-b", "ds-1", "sales"), publishedHit("connection-c", "ds-2", "other-sales")}},
		2: {PageIndex: 2, Limit: 2, Total: 5, Items: []tableausearch.Item{{LUID: "wb-2", Type: "workbook", Name: "Last"}}},
	}}
	resolver := &datasourceResolver{results: map[string]string{"sales": "ds-1", "other-sales": "ds-2"}}
	input := resourcesearch.Input{Types: []string{"datasource", "workbook"}, Terms: "sales", Limit: 2}
	first, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].LUID != "wb-1" || first.Items[1].LUID != "ds-1" || first.Items[1].Name != "Sales" || first.NextCursor == "" || first.Total != 0 {
		t.Fatalf("first=%+v", first)
	}
	// Resume in a fresh adapter, as a separate CLI process would do.
	input.Cursor = first.NextCursor
	second, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.Items[0].LUID != "ds-2" || second.Items[1].LUID != "wb-2" || second.NextCursor != "" || second.MoreAvailable || second.Total != 4 {
		t.Fatalf("second=%+v", second)
	}
}

func TestNativeAdapterFiltersEmbeddedConnectionsBeforeRESTResolution(t *testing.T) {
	embedded := publishedHit("embedded", "embedded-parent", "not-published")
	embedded.DatasourceIsPublished = new(false)
	embedded.ParentType = "Workbook"
	client := &nativeClient{pages: map[int]tableausearch.Page{0: {PageIndex: 0, Limit: 10, Total: 3,
		Items: []tableausearch.Item{embedded, publishedHit("a", "ds-1", "sales"), publishedHit("b", "ds-1", "sales")}}}}
	resolver := &datasourceResolver{results: map[string]string{"sales": "ds-1"}}
	page, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), resourcesearch.Input{Types: []string{"datasource"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 1 || !reflect.DeepEqual(resolver.calls, [][]string{{"sales"}}) {
		t.Fatalf("page=%+v calls=%v", page, resolver.calls)
	}
}

func TestNativeAdapterRejectsConflictingPublishedIdentity(t *testing.T) {
	client := &nativeClient{pages: map[int]tableausearch.Page{0: {PageIndex: 0, Limit: 10, Total: 1,
		Items: []tableausearch.Item{publishedHit("a", "wrong-parent", "sales")}}}}
	resolver := &datasourceResolver{results: map[string]string{"sales": "ds-1"}}
	if _, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), resourcesearch.Input{Types: []string{"datasource"}, Limit: 10}); err == nil {
		t.Fatal("accepted a search parent that disagrees with classic REST")
	}
}

func TestNativeAdapterTrailingConnectionsDoNotClaimMoreContent(t *testing.T) {
	client := &nativeClient{pages: map[int]tableausearch.Page{
		0: {PageIndex: 0, Limit: 1, Total: 2, HasNext: true, Items: []tableausearch.Item{publishedHit("a", "ds-1", "sales")}},
		1: {PageIndex: 1, Limit: 1, Total: 2, Items: []tableausearch.Item{publishedHit("b", "ds-1", "sales")}},
	}}
	resolver := &datasourceResolver{results: map[string]string{"sales": "ds-1"}}
	page, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), resourcesearch.Input{Types: []string{"datasource"}, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor != "" || page.MoreAvailable || page.Total != 1 || len(resolver.calls) != 1 {
		t.Fatalf("page=%+v error=%v resolver calls=%v", page, err, resolver.calls)
	}
}

func TestNativeAdapterResumeChecksPrefixAndReusesCommandPages(t *testing.T) {
	client := &nativeClient{pages: map[int]tableausearch.Page{
		0: {PageIndex: 0, Limit: 1, Total: 2, HasNext: true, Items: []tableausearch.Item{publishedHit("a", "ds-1", "sales")}},
		1: {PageIndex: 1, Limit: 1, Total: 2, Items: []tableausearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Workbook"}}},
	}}
	resolver := &datasourceResolver{results: map[string]string{"sales": "ds-1"}}
	input := resourcesearch.Input{Types: []string{"datasource", "workbook"}, Limit: 1}
	adapter := resourcesearch.NewNativeAdapter(client, resolver)
	first, err := adapter.Search(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Cursor = first.NextCursor
	if len(input.Cursor) > 4096 {
		t.Fatal("cursor exceeded its existing byte bound")
	}
	second, err := adapter.Search(t.Context(), input)
	if err != nil || len(second.Items) != 1 || second.Items[0].LUID != "wb-1" || len(client.requests) != 2 || len(resolver.calls) != 1 {
		t.Fatalf("second=%+v error=%v requests=%d resolutions=%d", second, err, len(client.requests), len(resolver.calls))
	}
	// A new process must detect changed earlier ranking/identity before resuming.
	client.pages[0].Items[0] = publishedHit("changed-hit", "ds-1", "sales")
	if _, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), input); err == nil {
		t.Fatal("resumed after the ranked prefix changed")
	}
	// Existing page-only cursors can still be read, reconstructing prior IDs.
	raw, err := base64.RawURLEncoding.DecodeString(first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	var cursor map[string]any
	if err := json.Unmarshal(raw, &cursor); err != nil {
		t.Fatal(err)
	}
	cursor["v"] = 1
	delete(cursor, "o")
	delete(cursor, "s")
	raw, err = json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	input.Cursor = base64.RawURLEncoding.EncodeToString(raw)
	legacy, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), input)
	if err != nil || len(legacy.Items) != 1 || legacy.Items[0].LUID != "wb-1" {
		t.Fatalf("legacy=%+v error=%v", legacy, err)
	}
}

func TestNativeAdapterConnectionScanRetainsNativeWindowLimit(t *testing.T) {
	client := &nativeClient{pages: make(map[int]tableausearch.Page)}
	for page := range 20 {
		items := make([]tableausearch.Item, 100)
		for offset := range items {
			items[offset] = publishedHit(fmt.Sprintf("connection-%d", page*100+offset), "ds-1", "sales")
		}
		client.pages[page] = tableausearch.Page{PageIndex: page, Limit: 100, Total: 2001, HasNext: page < 19, Items: items}
	}
	resolver := &datasourceResolver{results: map[string]string{"sales": "ds-1"}}
	page, err := resourcesearch.NewNativeAdapter(client, resolver).Search(t.Context(), resourcesearch.Input{Types: []string{"datasource"}, Limit: 100})
	if err != nil || len(page.Items) != 1 || !page.MoreAvailable || page.NextCursor != "" || page.Total != 0 || len(page.Warnings) != 1 || len(client.requests) != 20 || len(resolver.calls) != 1 {
		t.Fatalf("page=%+v error=%v requests=%d resolutions=%d", page, err, len(client.requests), len(resolver.calls))
	}
}

func TestNativeAdapterOverlappingPagesPreserveFirstRankAndQualifyCompleteness(t *testing.T) {
	first := tableausearch.Item{LUID: "wb-1", Type: "workbook", Name: "First"}
	client := &nativeClient{pages: map[int]tableausearch.Page{
		0: {PageIndex: 0, Limit: 2, Total: 4, HasNext: true, Items: []tableausearch.Item{first, {LUID: "wb-2", Type: "workbook", Name: "Second"}}},
		1: {PageIndex: 1, Limit: 2, Total: 4, Items: []tableausearch.Item{first, {LUID: "wb-3", Type: "workbook", Name: "Third"}}},
	}}
	adapter := resourcesearch.NewNativeAdapter(client, nil)
	input := resourcesearch.Input{Types: []string{"workbook"}, Limit: 2}
	page, err := adapter.Search(t.Context(), input)
	if err != nil || len(page.Items) != 2 || page.Items[0].LUID != "wb-1" || page.Items[1].LUID != "wb-2" || !page.UnresolvedMoreAvailable || len(page.Warnings) != 1 || page.Total != 0 {
		t.Fatalf("page=%+v error=%v", page, err)
	}
	input.Cursor = page.NextCursor
	last, err := adapter.Search(t.Context(), input)
	if err != nil || len(last.Items) != 1 || last.Items[0].LUID != "wb-3" || !last.UnresolvedMoreAvailable || !last.MoreAvailable || last.NextCursor != "" || last.Total != 0 {
		t.Fatalf("last=%+v error=%v", last, err)
	}
}
