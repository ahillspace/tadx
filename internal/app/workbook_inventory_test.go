package app

import (
	"context"
	"testing"

	workbookget "github.com/ahillspace/tadx/actions/workbook/inspect"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/identity"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type workbookInventoryAdapterStub struct {
	listRequest tableauworkbook.ListRequest
	selector    identity.Selector
	page        resourceworkbook.Page
	workbook    resourceworkbook.Workbook
}

func (a *workbookInventoryAdapterStub) ListWorkbooks(_ context.Context, request tableauworkbook.ListRequest) (resourceworkbook.Page, error) {
	a.listRequest = request
	return a.page, nil
}

func (a *workbookInventoryAdapterStub) ResolveWorkbook(_ context.Context, selector identity.Selector) (resourceworkbook.Workbook, error) {
	a.selector = selector
	return a.workbook, nil
}

func TestWorkbookListReaderMapsActionAndResourceTypes(t *testing.T) {
	adapter := &workbookInventoryAdapterStub{page: resourceworkbook.Page{
		Number: 1, Size: 25, Total: 1, RequestID: "request-1",
		Items: []resourceworkbook.Workbook{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-09-01T00:00:00Z", Tags: []string{"finance"}}},
	}}
	page, err := (workbookListReader{adapter: adapter}).ListWorkbooks(context.Background(), workbooklist.PageRequest{PageNumber: 1, PageSize: 25, Name: "Finance", OwnerName: "Analyst", ProjectLUID: "project-1", ProjectName: "Ops", Tag: "finance"})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.listRequest.Name != "Finance" || adapter.listRequest.OwnerName != "Analyst" || adapter.listRequest.ProjectLUID != "project-1" || adapter.listRequest.ProjectName != "Ops" || adapter.listRequest.Tag != "finance" {
		t.Fatalf("request = %#v", adapter.listRequest)
	}
	if page.RequestID != "request-1" || len(page.Workbooks) != 1 || page.Workbooks[0].ProjectPath != "Department/Ops" || page.Workbooks[0].Description != "Finance reporting" || len(page.Workbooks[0].Tags) != 1 {
		t.Fatalf("page = %#v", page)
	}
}

func TestWorkbookGetResolverMapsExactWorkbook(t *testing.T) {
	adapter := &workbookInventoryAdapterStub{workbook: resourceworkbook.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-09-01T00:00:00Z", Tags: []string{"finance"}, RequestID: "request-1"}}
	selector := identity.Selector{Name: "Finance", ProjectPath: "Department/Ops"}
	item, err := (workbookGetResolver{adapter: adapter}).ResolveWorkbook(context.Background(), selector)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.selector != selector {
		t.Fatalf("selector = %#v", adapter.selector)
	}
	want := workbookget.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-09-01T00:00:00Z", Tags: []string{"finance"}, RequestID: "request-1"}
	if item.LUID != want.LUID || item.ProjectPath != want.ProjectPath || item.RequestID != want.RequestID || item.Description != want.Description || len(item.Tags) != 1 {
		t.Fatalf("workbook = %#v", item)
	}
}
