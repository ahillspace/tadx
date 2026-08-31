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
	workbooks    map[string]tableauworkbook.Workbook
	getCalls     *int
	listCalls    *int
}

func (c client) Get(_ context.Context, luid string) (tableauworkbook.Workbook, error) {
	if c.getCalls != nil {
		(*c.getCalls)++
	}
	if item, ok := c.workbooks[luid]; ok {
		return item, nil
	}
	for _, page := range c.pages {
		for _, item := range page.Items {
			if item.LUID == luid {
				return item, nil
			}
		}
	}
	return tableauworkbook.Workbook{}, nil
}

func (c client) List(_ context.Context, page, _ int) (tableauworkbook.WorkbookPage, error) {
	if c.listCalls != nil {
		(*c.listCalls)++
	}
	return c.pages[page], nil
}

func (c client) ListProjects(_ context.Context, page, _ int) (tableauworkbook.ProjectPage, error) {
	return c.projectPages[page], nil
}

func (client) Download(context.Context, string, *bool) (tableauworkbook.Download, error) {
	return tableauworkbook.Download{}, nil
}

func (client) Prepare(context.Context, tableauworkbook.PublishRequest) (*tableauworkbook.PreparedPublish, error) {
	return nil, nil
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

func TestAdapterStopsAfterTwoDistinctWorkbookMatches(t *testing.T) {
	listCalls := 0
	adapter := resource.NewAdapter(client{listCalls: &listCalls, pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 2, Total: 10000}, Items: []tableauworkbook.Workbook{
			{LUID: "wb-1", Name: "Finance", ProjectName: "Ops"},
			{LUID: "wb-2", Name: "Finance", ProjectName: "Ops"},
		}},
	}})
	_, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance"})
	if err == nil || !strings.Contains(err.Error(), "wb-1, wb-2") {
		t.Fatalf("ResolveWorkbook() error = %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("list calls = %d", listCalls)
	}
}

func TestAdapterRejectsMatchingWorkbookWithoutLUID(t *testing.T) {
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 2, Total: 2}, Items: []tableauworkbook.Workbook{
			{LUID: "wb-1", Name: "Finance", ProjectName: "Ops"},
			{Name: "Finance", ProjectName: "Ops"},
		}},
	}})
	if _, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance"}); err == nil || !strings.Contains(err.Error(), "authoritative LUID") {
		t.Fatalf("ResolveWorkbook() error = %v", err)
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

func TestAdapterUsesExactGetForWorkbookLUID(t *testing.T) {
	getCalls, listCalls := 0, 0
	adapter := resource.NewAdapter(client{
		workbooks: map[string]tableauworkbook.Workbook{"wb-1": {LUID: "wb-1", Name: "Finance"}},
		getCalls:  &getCalls, listCalls: &listCalls,
	})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-1" || getCalls != 1 || listCalls != 0 {
		t.Fatalf("workbook = %#v, get calls = %d, list calls = %d", workbook, getCalls, listCalls)
	}
}

func TestAdapterDeduplicatesCollisionMatchesByWorkbookLUID(t *testing.T) {
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Workbook{
			{LUID: "wb-1", Name: "Finance", ContentURL: "finance", ProjectLUID: "project-1", ProjectName: "Ops"},
			{LUID: "wb-1", Name: "Finance", ContentURL: "finance", ProjectLUID: "project-1", ProjectName: "Ops"},
		}},
	}})
	matches, err := adapter.FindWorkbooks(context.Background(), "Finance", "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].LUID != "wb-1" {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestAdapterRejectsCollisionMatchWithoutWorkbookLUID(t *testing.T) {
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Workbook{
			{Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"},
		}},
	}})
	if _, err := adapter.FindWorkbooks(context.Background(), "Finance", "project-1"); err == nil || !strings.Contains(err.Error(), "authoritative LUID") {
		t.Fatalf("FindWorkbooks() error = %v", err)
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

func TestAdapterRejectsProjectWithoutLUID(t *testing.T) {
	adapter := resource.NewAdapter(client{projectPages: map[int]tableauworkbook.ProjectPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 1, Total: 1}, Items: []tableauworkbook.Project{{Name: "Ops"}}},
	}})
	if _, err := adapter.ResolveProject(context.Background(), identity.Selector{ProjectPath: "Ops"}); err == nil || !strings.Contains(err.Error(), "authoritative LUID") {
		t.Fatalf("ResolveProject() error = %v", err)
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
