package workbook_test

import (
	"fmt"
	"strings"
	"testing"

	resource "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func TestCollectProjectWorkbooksAcceptsMaximumAndDoesNotReuseResults(t *testing.T) {
	client := newProjectCollectionClient(10_000)
	adapter := resource.NewAdapterWithProjectResolver(client, projectPaths{"project-1": "Ops"})
	request := tableauworkbook.ListRequest{ProjectLUID: "project-1"}

	for call := range 2 {
		page, err := adapter.CollectProjectWorkbooks(t.Context(), request)
		if err != nil {
			t.Fatalf("call %d: %v", call+1, err)
		}
		if page.Total != 10_000 || len(page.Items) != 10_000 || page.Items[0].LUID != "wb-00000" || page.Items[9_999].LUID != "wb-09999" {
			t.Fatalf("call %d returned unexpected collection: total=%d items=%d first=%q last=%q", call+1, page.Total, len(page.Items), page.Items[0].LUID, page.Items[len(page.Items)-1].LUID)
		}
		if len(client.requests) != (call+1)*10 {
			t.Fatalf("call %d made %d list requests, want %d", call+1, len(client.requests), (call+1)*10)
		}
	}
}

func TestCollectProjectWorkbooksRejectsAboveMaximum(t *testing.T) {
	client := newProjectCollectionClient(10_001)
	adapter := resource.NewAdapterWithProjectResolver(client, projectPaths{"project-1": "Ops"})
	_, err := adapter.CollectProjectWorkbooks(t.Context(), tableauworkbook.ListRequest{ProjectLUID: "project-1"})
	if err == nil || !strings.Contains(err.Error(), "10000-record bound") {
		t.Fatalf("error = %v, want bounded collection error", err)
	}
	if len(client.requests) != 11 {
		t.Fatalf("list requests = %d, want 11 to establish the over-limit match count", len(client.requests))
	}
}

func TestCollectProjectWorkbooksRejectsMissingIdentity(t *testing.T) {
	client := newProjectCollectionClient(1)
	page := client.pages[1]
	page.Items[0].LUID = ""
	client.pages[1] = page
	adapter := resource.NewAdapterWithProjectResolver(client, projectPaths{"project-1": "Ops"})
	_, err := adapter.CollectProjectWorkbooks(t.Context(), tableauworkbook.ListRequest{ProjectLUID: "project-1"})
	if err == nil || !strings.Contains(err.Error(), "authoritative LUID") {
		t.Fatalf("error = %v, want missing authoritative LUID error", err)
	}
}

func newProjectCollectionClient(total int) *projectFilterInventoryClient {
	const pageSize = 1000
	pages := make(map[int]tableauworkbook.WorkbookPage, (total+pageSize-1)/pageSize)
	for pageNumber := 1; pageNumber <= (total+pageSize-1)/pageSize; pageNumber++ {
		start := (pageNumber - 1) * pageSize
		end := min(start+pageSize, total)
		items := make([]tableauworkbook.Workbook, 0, end-start)
		for index := start; index < end; index++ {
			items = append(items, tableauworkbook.Workbook{
				LUID:        fmt.Sprintf("wb-%05d", index),
				Name:        fmt.Sprintf("Workbook %05d", index),
				ProjectLUID: "project-1",
				ProjectName: "Ops",
			})
		}
		pages[pageNumber] = tableauworkbook.WorkbookPage{
			Page:  tableauworkbook.Page{Number: pageNumber, Size: pageSize, Total: total},
			Items: items,
		}
	}
	return &projectFilterInventoryClient{pages: pages}
}
