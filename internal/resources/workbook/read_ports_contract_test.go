package workbook_test

import (
	"encoding/json"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
	resource "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func TestReadPortsMapActionRequestAndRichPage(t *testing.T) {
	client := &inventoryClient{page: tableauworkbook.WorkbookPage{
		Page: tableauworkbook.Page{Number: 1, Size: 1000, Total: 1}, TableauRequestID: "request-1",
		Items: []tableauworkbook.Workbook{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops", ContentURL: "Finance", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-09-01T00:00:00Z", Tags: []string{"finance"}}},
	}}
	ports := resource.ReadPorts{Adapter: resource.NewAdapterWithProjectResolver(client, projectPaths{"project-1": "Department/Ops"})}
	page, err := ports.ListWorkbooks(t.Context(), workbookops.ListPageRequest{PageNumber: 1, PageSize: 25, Name: "Finance", OwnerName: "Analyst", ProjectLUID: "project-1", ProjectName: "Ops", Tag: "finance"})
	if err != nil {
		t.Fatal(err)
	}
	if client.request.Name != "Finance" || client.request.OwnerName != "Analyst" || client.request.ProjectLUID != "project-1" || client.request.ProjectName != "Ops" || client.request.Tag != "finance" {
		t.Fatalf("request = %#v", client.request)
	}
	if page.RequestID != "request-1" || len(page.Workbooks) != 1 || page.Workbooks[0].ProjectPath != "Department/Ops" || page.Workbooks[0].Description != "Finance reporting" || len(page.Workbooks[0].Tags) != 1 {
		t.Fatalf("page = %#v", page)
	}
}

func TestReadPortsInspectPreservesResourceRecord(t *testing.T) {
	native := tableauworkbook.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ContentURL: "Finance", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-09-01T00:00:00Z", Tags: []string{"finance"}, TableauRequestID: "request-1"}
	ports := resource.ReadPorts{Adapter: resource.NewAdapterWithProjectResolver(client{workbooks: map[string]tableauworkbook.Workbook{"wb-1": native}}, projectPaths{"project-1": "Department/Ops"})}
	item, err := ports.ResolveWorkbook(t.Context(), identity.Selector{LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != "wb-1" || item.ProjectPath != "Department/Ops" || item.RequestID != "request-1" || item.Description != "Finance reporting" || len(item.Tags) != 1 {
		t.Fatalf("workbook = %#v", item)
	}
}

func TestWorkbookRecordPreservesCachedInspectPayload(t *testing.T) {
	item := workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "p-1", OwnerLUID: "owner", Tags: []string{"finance"}, RequestID: "private-request"}
	encoded, err := json.Marshal(item)
	want := `{"luid":"wb-1","name":"Finance","project_luid":"p-1","project_path":"","owner_luid":"owner","tags":["finance"]}`
	if err != nil || string(encoded) != want {
		t.Fatalf("cache payload=%s err=%v, want %s", encoded, err, want)
	}
}
