package workbook_test

import (
	"context"
	"fmt"
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

func (c client) Delete(ctx context.Context, luid string) (tableauworkbook.MutationResult, error) {
	return tableauworkbook.MutationResult{Status: "succeeded", WorkbookLUID: luid, TableauRequestID: "request-1"}, nil
}

type inventoryClient struct {
	client
	page    tableauworkbook.WorkbookPage
	request tableauworkbook.ListRequest
}

func (c *inventoryClient) ListWorkbooks(_ context.Context, request tableauworkbook.ListRequest) (tableauworkbook.WorkbookPage, error) {
	c.request = request
	return c.page, nil
}

type projectPaths map[string]string

func (p projectPaths) ResolveProjectPath(_ context.Context, luid string) (string, error) {
	return p[luid], nil
}

type batchedProjectPaths struct {
	projectPaths
	calls int
}

func (p *batchedProjectPaths) ResolveProjectPaths(_ context.Context, luids []string) (map[string]string, error) {
	p.calls++
	result := make(map[string]string, len(luids))
	for _, luid := range luids {
		path, ok := p.projectPaths[luid]
		if !ok {
			return nil, fmt.Errorf("missing project %s", luid)
		}
		result[luid] = path
	}
	return result, nil
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

func TestAdapterListsOneBoundedWorkbookPage(t *testing.T) {
	c := &inventoryClient{page: tableauworkbook.WorkbookPage{
		Page: tableauworkbook.Page{Number: 1, Size: 25, Total: 1}, TableauRequestID: "request-1",
		Items: []tableauworkbook.Workbook{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops", Description: "Finance reporting", Tags: []string{"finance"}}},
	}}
	page, err := resource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "Department/Ops"}).ListWorkbooks(context.Background(), tableauworkbook.ListRequest{PageNumber: 1, PageSize: 25, Name: "Finance"})
	if err != nil {
		t.Fatal(err)
	}
	if c.request.Name != "Finance" || page.RequestID != "request-1" || len(page.Items) != 1 || page.Items[0].ProjectPath != "Department/Ops" || page.Items[0].Description != "Finance reporting" {
		t.Fatalf("page = %#v, request = %#v", page, c.request)
	}
}

func TestAdapterListsWorkbookPageWithOneProjectHierarchyResolution(t *testing.T) {
	c := &inventoryClient{page: tableauworkbook.WorkbookPage{
		Page: tableauworkbook.Page{Number: 1, Size: 25, Total: 2},
		Items: []tableauworkbook.Workbook{
			{LUID: "wb-1", Name: "One", ProjectLUID: "project-1"},
			{LUID: "wb-2", Name: "Two", ProjectLUID: "project-2"},
		},
	}}
	paths := &batchedProjectPaths{projectPaths: projectPaths{"project-1": "Department/Ops", "project-2": "Shared"}}
	page, err := resource.NewAdapterWithProjectResolver(c, paths).ListWorkbooks(context.Background(), tableauworkbook.ListRequest{PageNumber: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if paths.calls != 1 || page.Items[0].ProjectPath != "Department/Ops" || page.Items[1].ProjectPath != "Shared" {
		t.Fatalf("batch calls = %d, page = %#v", paths.calls, page)
	}
}

func TestAdapterRejectsInvalidWorkbookPageInput(t *testing.T) {
	c := &inventoryClient{page: tableauworkbook.WorkbookPage{Page: tableauworkbook.Page{Number: 1, Size: 25, Total: 0}}}
	adapter := resource.NewAdapterWithProjectResolver(c, projectPaths{})
	tests := []struct {
		name    string
		request tableauworkbook.ListRequest
		wantErr string
	}{
		{name: "zero page number", request: tableauworkbook.ListRequest{PageNumber: 0, PageSize: 25}, wantErr: "page number must be positive"},
		{name: "negative page number", request: tableauworkbook.ListRequest{PageNumber: -1, PageSize: 25}, wantErr: "page number must be positive"},
		{name: "zero page size", request: tableauworkbook.ListRequest{PageNumber: 1, PageSize: 0}, wantErr: "page size must be between 1 and 1000"},
		{name: "oversized page", request: tableauworkbook.ListRequest{PageNumber: 1, PageSize: 1001}, wantErr: "page size must be between 1 and 1000"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := adapter.ListWorkbooks(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "inconsistent pagination") {
				t.Fatalf("input error masked as pagination error: %v", err)
			}
		})
	}
}

func TestAdapterUsesSharedProjectResolverForExactWorkbook(t *testing.T) {
	c := client{workbooks: map[string]tableauworkbook.Workbook{"wb-1": {LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops", TableauRequestID: "request-1"}}}
	workbook, err := resource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "Department/Ops"}).ResolveWorkbook(context.Background(), identity.Selector{LUID: "wb-1"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.ProjectPath != "Department/Ops" || workbook.RequestID != "request-1" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterRejectsWorkbookPaginationDrift(t *testing.T) {
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 1, Total: 2}, Items: []tableauworkbook.Workbook{{LUID: "wb-1", Name: "Other"}}},
		2: {Page: tableauworkbook.Page{Number: 2, Size: 1, Total: 3}, Items: []tableauworkbook.Workbook{{LUID: "wb-2", Name: "Other"}}},
	}})
	_, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance"})
	if err == nil || !strings.Contains(err.Error(), "pagination total changed") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdapterResolvesExactWorkbookAcrossAllPages(t *testing.T) {
	adapter := resource.NewAdapter(client{
		pages: map[int]tableauworkbook.WorkbookPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 1, Total: 2}, Items: []tableauworkbook.Workbook{{LUID: "wb-1", Name: "Other", ProjectName: "Ops"}}},
			2: {Page: tableauworkbook.Page{Number: 2, Size: 1, Total: 2}, Items: []tableauworkbook.Workbook{{LUID: "wb-2", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"}}},
		},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Project{{LUID: "project-1", Name: "Ops"}}},
		},
	})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance", ProjectPath: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-2" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterResolvesExactWorkbookByProjectLUID(t *testing.T) {
	c := &inventoryClient{page: tableauworkbook.WorkbookPage{
		Page: tableauworkbook.Page{Number: 1, Size: 2, Total: 2},
		Items: []tableauworkbook.Workbook{
			{LUID: "wb-other", Name: "Finance", ProjectLUID: "project-2", ProjectName: "Other"},
			{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"},
		},
	}}
	workbook, err := resource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "Department/Ops", "project-2": "Other"}).ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance", ProjectLUID: "project-1"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-1" || workbook.ProjectPath != "Department/Ops" || c.request.ProjectLUID != "project-1" {
		t.Fatalf("workbook = %#v, request = %#v", workbook, c.request)
	}
}

func TestAdapterHardFailsAmbiguousWorkbookSelector(t *testing.T) {
	adapter := resource.NewAdapter(client{
		pages: map[int]tableauworkbook.WorkbookPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Workbook{
				{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"},
				{LUID: "wb-2", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"},
			}},
		},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Project{{LUID: "project-1", Name: "Ops"}}},
		},
	})
	_, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance", ProjectPath: "Ops"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdapterStopsAfterTwoDistinctWorkbookMatches(t *testing.T) {
	listCalls := 0
	adapter := resource.NewAdapter(client{listCalls: &listCalls, pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 2, Total: 10000}, Items: []tableauworkbook.Workbook{
			{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"},
			{LUID: "wb-2", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"},
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
			{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"},
			{Name: "Finance", ProjectName: "Ops"},
		}},
	}})
	if _, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance"}); err == nil || !strings.Contains(err.Error(), "authoritative LUID") {
		t.Fatalf("ResolveWorkbook() error = %v", err)
	}
}

func TestAdapterRejectsMatchingWorkbookWithoutProjectLUID(t *testing.T) {
	item := tableauworkbook.Workbook{LUID: "wb-1", Name: "Finance", ProjectName: "Ops"}
	tests := []struct {
		name     string
		client   client
		selector identity.Selector
	}{
		{
			name: "name selector",
			client: client{pages: map[int]tableauworkbook.WorkbookPage{
				1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Workbook{item}},
			}},
			selector: identity.Selector{Name: "Finance"},
		},
		{
			name:     "LUID selector",
			client:   client{workbooks: map[string]tableauworkbook.Workbook{"wb-1": item}},
			selector: identity.Selector{LUID: "wb-1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := resource.NewAdapter(test.client)
			if _, err := adapter.ResolveWorkbook(context.Background(), test.selector); err == nil || !strings.Contains(err.Error(), "project LUID") {
				t.Fatalf("ResolveWorkbook() error = %v", err)
			}
		})
	}
}

func TestAdapterReturnsIdentityWinnerForDuplicateWorkbookLUID(t *testing.T) {
	adapter := resource.NewAdapter(client{
		pages: map[int]tableauworkbook.WorkbookPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Workbook{
				{LUID: "wb-1", Name: "Finance", ContentURL: "finance", ProjectLUID: "project-new", ProjectName: "New"},
				{LUID: "wb-1", Name: "Renamed Finance", ContentURL: "renamed-finance", ProjectLUID: "project-old", ProjectName: "Old"},
			}},
		},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Project{
				{LUID: "project-new", Name: "New"},
				{LUID: "project-old", Name: "Old"},
			}},
		},
	})
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
		workbooks: map[string]tableauworkbook.Workbook{"wb-1": {LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectName: "Ops"}},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Project{{LUID: "project-1", Name: "Ops"}}},
		},
		getCalls: &getCalls, listCalls: &listCalls,
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

func TestAdapterRejectsIncompleteSameNameCollisionIdentityBeforeProjectFiltering(t *testing.T) {
	tests := []struct {
		name      string
		item      tableauworkbook.Workbook
		errorText string
	}{
		{
			name:      "missing workbook LUID in another project",
			item:      tableauworkbook.Workbook{Name: "Finance", ProjectLUID: "project-other", ProjectName: "Other"},
			errorText: "authoritative LUID",
		},
		{
			name:      "missing project LUID",
			item:      tableauworkbook.Workbook{LUID: "wb-1", Name: "Finance", ProjectName: "Ops"},
			errorText: "project LUID",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
				1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Workbook{test.item}},
			}})
			if _, err := adapter.FindWorkbooks(context.Background(), "Finance", "project-1"); err == nil || !strings.Contains(err.Error(), test.errorText) {
				t.Fatalf("FindWorkbooks() error = %v", err)
			}
		})
	}
}

func TestAdapterRejectsConflictingDuplicateWorkbookLUIDRowsRegardlessOfOrder(t *testing.T) {
	first := tableauworkbook.Workbook{LUID: "wb-1", Name: "Finance", ContentURL: "finance", ProjectLUID: "project-a", ProjectName: "Ops", OwnerLUID: "owner-1"}
	second := first
	second.ProjectLUID = "project-b"
	for _, items := range [][]tableauworkbook.Workbook{{first, second}, {second, first}} {
		adapter := resource.NewAdapter(client{
			pages: map[int]tableauworkbook.WorkbookPage{
				1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: items},
			},
			projectPages: map[int]tableauworkbook.ProjectPage{
				1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Project{
					{LUID: "project-a", Name: "Ops"},
					{LUID: "project-b", Name: "Ops"},
				}},
			},
		})
		if _, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance"}); err == nil || !strings.Contains(err.Error(), "conflicting records") {
			t.Fatalf("ResolveWorkbook() error = %v", err)
		}
	}
}

func TestAdapterDeduplicatesEquivalentWorkbookLUIDRows(t *testing.T) {
	item := tableauworkbook.Workbook{LUID: "wb-1", Name: "Finance", ContentURL: "finance", ProjectLUID: "project-1", ProjectName: "Ops", OwnerLUID: "owner-1"}
	adapter := resource.NewAdapter(client{
		pages: map[int]tableauworkbook.WorkbookPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Workbook{item, item}},
		},
		projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 1}, Items: []tableauworkbook.Project{{LUID: "project-1", Name: "Ops"}}},
		},
	})
	workbook, err := adapter.ResolveWorkbook(context.Background(), identity.Selector{Name: "Finance"})
	if err != nil {
		t.Fatal(err)
	}
	if workbook.LUID != "wb-1" || workbook.ProjectPath != "Ops" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestAdapterRejectsConflictingCollisionRowsWithSameWorkbookLUID(t *testing.T) {
	first := tableauworkbook.Workbook{LUID: "wb-1", Name: "Finance", ContentURL: "finance", ProjectLUID: "project-1", ProjectName: "Ops", OwnerLUID: "owner-1"}
	second := first
	second.OwnerLUID = "owner-2"
	adapter := resource.NewAdapter(client{pages: map[int]tableauworkbook.WorkbookPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 2}, Items: []tableauworkbook.Workbook{first, second}},
	}})
	if _, err := adapter.FindWorkbooks(context.Background(), "Finance", "project-1"); err == nil || !strings.Contains(err.Error(), "conflicting records") {
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

func TestAdapterRejectsConflictingDuplicateProjectLUIDRowsRegardlessOfOrder(t *testing.T) {
	first := tableauworkbook.Project{LUID: "child", Name: "Ops", ParentLUID: "parent-a"}
	second := first
	second.ParentLUID = "parent-b"
	for _, duplicates := range [][]tableauworkbook.Project{{first, second}, {second, first}} {
		items := []tableauworkbook.Project{
			{LUID: "parent-a", Name: "Department A"},
			{LUID: "parent-b", Name: "Department B"},
		}
		items = append(items, duplicates...)
		adapter := resource.NewAdapter(client{projectPages: map[int]tableauworkbook.ProjectPage{
			1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: len(items)}, Items: items},
		}})
		if _, err := adapter.ResolveProject(context.Background(), identity.Selector{LUID: "child"}); err == nil || !strings.Contains(err.Error(), "conflicting records") {
			t.Fatalf("ResolveProject() error = %v", err)
		}
	}
}

func TestAdapterDeduplicatesEquivalentProjectLUIDRows(t *testing.T) {
	child := tableauworkbook.Project{LUID: "child", Name: "Ops", ParentLUID: "parent"}
	adapter := resource.NewAdapter(client{projectPages: map[int]tableauworkbook.ProjectPage{
		1: {Page: tableauworkbook.Page{Number: 1, Size: 100, Total: 3}, Items: []tableauworkbook.Project{
			{LUID: "parent", Name: "Department"},
			child,
			child,
		}},
	}})
	project, err := adapter.ResolveProject(context.Background(), identity.Selector{LUID: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if project.Path != "Department/Ops" {
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
