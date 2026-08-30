package workbook_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	resource "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type client struct {
	pages        map[int]tableauworkbook.WorkbookPage
	projectPages map[int]tableauworkbook.ProjectPage
}

func (c client) List(_ context.Context, page, _ int) (tableauworkbook.WorkbookPage, error) {
	return c.pages[page], nil
}

func (c client) ListProjects(_ context.Context, page, _ int) (tableauworkbook.ProjectPage, error) {
	return c.projectPages[page], nil
}

func (client) Download(context.Context, string, *bool) (tableauworkbook.Download, error) {
	return tableauworkbook.Download{}, nil
}

func (client) Publish(context.Context, tableauworkbook.PublishRequest) (tableauworkbook.PublishResult, error) {
	return tableauworkbook.PublishResult{}, nil
}

func TestAdapterResolvesExactWorkbookAcrossAllPages(t *testing.T) {
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 1, Total: 2}, Items: []tableauworkbook.Workbook{{LUID: "wb-1", Name: "Other", ProjectName: "Ops"}}},
		2: {Page: tableauworkbook.Page{Number: 2, Size: 1, Total: 2}, Items: []tableauworkbook.Workbook{{LUID: "wb-2", Name: "Finance", ProjectName: "Ops"}}},
	}})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance", ProjectPath: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-2" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterHardFailsAmbiguousWorkbookSelector(t *testing.T) {
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Workbook{
			{LUID: "wb-1", Name: "Finance", ProjectName: "Ops"},
			{LUID: "wb-2", Name: "Finance", ProjectName: "Ops"},
		}},
	}})
	_, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance", ProjectPath: "Ops"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdapterReturnsIdentityWinnerForDuplicateWorkbookLUID(t *testing.T) {
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Workbook{
			{LUID: "wb-1", Name: "Finance", ContentURL: "finance", ProjectName: "New"},
			{LUID: "wb-1", Name: "Renamed Finance", ContentURL: "renamed-finance", ProjectName: "Old"},
		}},
	}})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.Name != "Finance" || workbook.ContentURL != "finance" || workbook.ProjectPath != "New" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterResolvesExactNestedProjectPath(t *testing.T) {
	adapter := resource.NewAdapter(client{projectPages: map[int]tableauworkbook.ProjectPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Project{
			{LUID: "parent", Name: "Department"},
			{LUID: "child", Name: "Ops", ParentLUID: "parent"},
		}},
	}})
	project, err := adapter.ResolveProject(context.Background(), identity.Selector{ProjectPath: "Department/Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if project.LUID != "child" || project.Path != "Department/Ops" {
		t.Fatalf("project = %#v", project)
	}
}

func TestAdapterResolvesWorkbookInExactNestedProjectPath(t *testing.T) {
	adapter := resource.NewAdapter(client{
		pages: map[int]tableauworkbook.WorkbookPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Workbook{
				{LUID: "wb-1", Name: "Finance", ProjectLUID: "child", ProjectName: "Ops"},
			}},
		},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Project{
				{LUID: "parent", Name: "Department"},
				{LUID: "child", Name: "Ops", ParentLUID: "parent"},
			}},
		},
	})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance", ProjectPath: "Department/Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-1" || workbook.ProjectPath != "Department/Ops" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterReportsNestedProjectPathForWorkbookLUID(t *testing.T) {
	adapter := resource.NewAdapter(client{
		pages: map[int]tableauworkbook.WorkbookPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Workbook{
				{LUID: "wb-1", Name: "Finance", ProjectLUID: "child", ProjectName: "Ops"},
			}},
		},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 3}, Items: []tableauworkbook.Project{
				{LUID: "broken", Name: "Broken", ParentLUID: "missing"},
				{LUID: "parent", Name: "Department"},
				{LUID: "child", Name: "Ops", ParentLUID: "parent"},
			}},
		},
	})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-1" || workbook.ProjectPath != "Department/Ops" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterReportsNestedProjectPathForUniqueWorkbookName(t *testing.T) {
	adapter := resource.NewAdapter(client{
		pages: map[int]tableauworkbook.WorkbookPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Workbook{
				{LUID: "wb-1", Name: "Finance", ProjectLUID: "child", ProjectName: "Ops"},
			}},
		},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 3}, Items: []tableauworkbook.Project{
				{LUID: "broken", Name: "Broken", ParentLUID: "missing"},
				{LUID: "parent", Name: "Department"},
				{LUID: "child", Name: "Ops", ParentLUID: "parent"},
			}},
		},
	})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-1" || workbook.ProjectPath != "Department/Ops" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterResolvesProjectLUIDWithoutUnrelatedHierarchy(t *testing.T) {
	adapter := resource.NewAdapter(client{projectPages: map[int]tableauworkbook.ProjectPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 3}, Items: []tableauworkbook.Project{
			{LUID: "broken", Name: "Broken", ParentLUID: "missing"},
			{LUID: "parent", Name: "Department"},
			{LUID: "child", Name: "Ops", ParentLUID: "parent"},
		}},
	}})
	project, err := adapter.ResolveProject(context.Background(), identity.Selector{LUID: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if project.LUID != "child" || project.Path != "Department/Ops" {
		t.Fatalf("project = %#v", project)
	}
}
