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
