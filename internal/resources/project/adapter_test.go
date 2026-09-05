package project_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

type projectClient struct {
	pages map[int]tableauproject.Page
	calls []tableauproject.ListRequest
}

func (c *projectClient) List(_ context.Context, input tableauproject.ListRequest) (tableauproject.Page, error) {
	c.calls = append(c.calls, input)
	return c.pages[input.PageNumber], nil
}

func TestAdapterListsOneBoundedPage(t *testing.T) {
	client := &projectClient{pages: map[int]tableauproject.Page{2: {
		Number: 2, Size: 2, Total: 5,
		Items: []tableauproject.Project{{LUID: "p-3", Name: "Three", ParentLUID: "root"}, {LUID: "p-4", Name: "Four", ParentLUID: "root"}},
	}}}
	page, err := resourceproject.NewAdapter(client).ListProjects(context.Background(), resourceproject.ListRequest{PageNumber: 2, PageSize: 2, Name: "Four"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 2 || page.Total != 5 || len(page.Items) != 2 || page.Items[0].LUID != "p-3" {
		t.Fatalf("page = %#v", page)
	}
	if len(client.calls) != 1 || client.calls[0].Name != "Four" {
		t.Fatalf("calls = %#v", client.calls)
	}
}

func TestImportedSelectorAliasPreservesRealProjectCollisions(t *testing.T) {
	tests := []struct {
		name, selector, realName, want string
		wantError                      bool
	}{
		{name: "display label", selector: "Imported", want: "(imported)"},
		{name: "lowercase label", selector: "imported", want: "(imported)"},
		{name: "literal label", selector: "(imported)", want: "(imported)"},
		{name: "real exact project wins", selector: "Imported", realName: "Imported", want: "Imported"},
		{name: "real lowercase project wins", selector: "imported", realName: "imported", want: "imported"},
		{name: "real differently cased project blocks alias", selector: "imported", realName: "Imported", wantError: true},
		{name: "unrelated path stays exact", selector: "Import", wantError: true},
		{name: "nested path is not normalized", selector: "Team/Imported", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []tableauproject.Project{{LUID: "system-project", Name: "(imported)"}}
			if tt.realName != "" {
				items = append(items, tableauproject.Project{LUID: "real-project", Name: tt.realName})
			}
			client := &projectClient{pages: map[int]tableauproject.Page{1: {Number: 1, Size: 1000, Total: len(items), Items: items}}}
			path, err := resourceproject.NewAdapter(client).ResolveProjectSelectorPath(context.Background(), tt.selector)
			if tt.wantError {
				if err == nil {
					t.Fatalf("unsafe selector resolved to %q", path)
				}
			} else if err != nil || path != tt.want {
				t.Fatalf("path=%q err=%v", path, err)
			}
			if len(client.calls) != 1 {
				t.Fatalf("calls=%d", len(client.calls))
			}
		})
	}
}

func TestAdapterResolvesNestedPathAcrossPages(t *testing.T) {
	client := &projectClient{pages: map[int]tableauproject.Page{
		1: {Number: 1, Size: 2, Total: 3, Items: []tableauproject.Project{{LUID: "root", Name: "Department"}, {LUID: "other", Name: "Ops"}}},
		2: {Number: 2, Size: 2, Total: 3, Items: []tableauproject.Project{{LUID: "child", Name: "Ops", ParentLUID: "root"}}},
	}}
	project, err := resourceproject.NewAdapter(client).ResolveProject(context.Background(), identity.Selector{ProjectPath: "Department/Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if project.LUID != "child" || project.Path != "Department/Ops" || project.ParentLUID != "root" {
		t.Fatalf("project = %#v", project)
	}
}

func TestAdapterRejectsConflictingProjectRowsAndCycles(t *testing.T) {
	for name, pages := range map[string]map[int]tableauproject.Page{
		"conflict": {
			1: {Number: 1, Size: 2, Total: 2, Items: []tableauproject.Project{{LUID: "p", Name: "One"}, {LUID: "p", Name: "Two"}}},
		},
		"cycle": {
			1: {Number: 1, Size: 2, Total: 2, Items: []tableauproject.Project{{LUID: "a", Name: "A", ParentLUID: "b"}, {LUID: "b", Name: "B", ParentLUID: "a"}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resourceproject.NewAdapter(&projectClient{pages: pages}).ResolveProject(context.Background(), identity.Selector{LUID: "a", ProjectPath: "A"})
			if err == nil || (!strings.Contains(err.Error(), "conflicting") && !strings.Contains(err.Error(), "cycle")) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAdapterRejectsProjectNameContainingPathSeparator(t *testing.T) {
	client := &projectClient{pages: map[int]tableauproject.Page{
		1: {Number: 1, Size: 2, Total: 1, Items: []tableauproject.Project{{LUID: "p", Name: "Ops/Reports"}}},
	}}
	adapter := resourceproject.NewAdapter(client)
	if _, err := adapter.ResolveProject(context.Background(), identity.Selector{ProjectPath: "Ops/Reports"}); err == nil || !strings.Contains(err.Error(), "not addressable by an exact project path") {
		t.Fatalf("ResolveProject error = %v", err)
	}
	if _, err := adapter.ListProjects(context.Background(), resourceproject.ListRequest{PageNumber: 1, PageSize: 2}); err == nil || !strings.Contains(err.Error(), "not addressable by an exact project path") {
		t.Fatalf("ListProjects error = %v", err)
	}
}

func TestAdapterRejectsChangingProjectPagination(t *testing.T) {
	client := &projectClient{pages: map[int]tableauproject.Page{
		1: {Number: 1, Size: 1, Total: 2, Items: []tableauproject.Project{{LUID: "root", Name: "Department"}}},
		2: {Number: 2, Size: 1, Total: 3, Items: []tableauproject.Project{{LUID: "child", Name: "Ops", ParentLUID: "root"}}},
	}}
	_, err := resourceproject.NewAdapter(client).ResolveProject(context.Background(), identity.Selector{ProjectPath: "Department/Ops"})
	if err == nil || !strings.Contains(err.Error(), "pagination total changed") {
		t.Fatalf("error = %v", err)
	}
}
