package project_test

import (
	"context"
	"testing"

	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

type projectMutationClient struct {
	create tableauproject.CreateRequest
	update tableauproject.UpdateRequest
	delete string
	calls  int
}

func (c *projectMutationClient) Create(_ context.Context, input tableauproject.CreateRequest) (tableauproject.MutationResult, error) {
	c.calls++
	c.create = input
	return tableauproject.MutationResult{Status: "succeeded", Project: tableauproject.Project{LUID: "project-1", Name: input.Name, ParentLUID: input.ParentLUID}}, nil
}

func (c *projectMutationClient) Update(_ context.Context, input tableauproject.UpdateRequest) (tableauproject.MutationResult, error) {
	c.calls++
	c.update = input
	return tableauproject.MutationResult{Status: "succeeded", Project: tableauproject.Project{LUID: input.LUID, Name: "Renamed"}}, nil
}

func (c *projectMutationClient) Delete(_ context.Context, luid string) (tableauproject.DeleteResult, error) {
	c.calls++
	c.delete = luid
	return tableauproject.DeleteResult{Status: "succeeded", ProjectLUID: luid}, nil
}

func TestMutationAdapterValidatesAndDelegatesExactProjectMutations(t *testing.T) {
	client := &projectMutationClient{}
	adapter := resourceproject.NewMutationAdapter(client)
	created, err := adapter.CreateProject(context.Background(), tableauproject.CreateRequest{Name: "Operations", ParentLUID: "parent-1"})
	if err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	updated, err := adapter.UpdateProject(context.Background(), tableauproject.UpdateRequest{LUID: "project-1", Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := adapter.DeleteProject(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 || client.create.ParentLUID != "parent-1" || client.update.LUID != "project-1" || client.delete != "project-1" || created.Project.LUID != "project-1" || updated.Project.Name != "Renamed" || deleted.ProjectLUID != "project-1" {
		t.Fatalf("client=%#v created=%#v updated=%#v", client, created, updated)
	}
}

func TestMutationAdapterRejectsIncompleteInputs(t *testing.T) {
	client := &projectMutationClient{}
	adapter := resourceproject.NewMutationAdapter(client)
	if _, err := adapter.CreateProject(context.Background(), tableauproject.CreateRequest{}); err == nil {
		t.Fatal("CreateProject() accepted a missing name")
	}
	if _, err := adapter.UpdateProject(context.Background(), tableauproject.UpdateRequest{}); err == nil {
		t.Fatal("UpdateProject() accepted a missing LUID and fields")
	}
	if _, err := adapter.DeleteProject(context.Background(), ""); err == nil {
		t.Fatal("DeleteProject() accepted a missing LUID")
	}
	if client.calls != 0 {
		t.Fatalf("calls = %d", client.calls)
	}
}

func TestAdapterFindsCaseInsensitiveSiblingCollisionOnly(t *testing.T) {
	client := &projectClient{pages: map[int]tableauproject.Page{1: {
		Number: 1, Size: 4, Total: 4,
		Items: []tableauproject.Project{
			{LUID: "root-a", Name: "A"},
			{LUID: "root-b", Name: "B"},
			{LUID: "child-a", Name: "Operations", ParentLUID: "root-a"},
			{LUID: "child-b", Name: "operations", ParentLUID: "root-b"},
		},
	}}}
	adapter := resourceproject.NewAdapter(client)
	matches, err := adapter.FindProjectCollisions(context.Background(), "OPERATIONS", "root-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].LUID != "child-a" || matches[0].Path != "A/Operations" {
		t.Fatalf("matches = %#v", matches)
	}
	rootMatches, err := adapter.FindProjectCollisions(context.Background(), "a", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootMatches) != 1 || rootMatches[0].LUID != "root-a" {
		t.Fatalf("root matches = %#v", rootMatches)
	}
}

func TestAdapterNormalizesMutationProjectWithoutRequeryingNewIdentity(t *testing.T) {
	client := &projectClient{pages: map[int]tableauproject.Page{1: {
		Number: 1, Size: 1, Total: 1, Items: []tableauproject.Project{{LUID: "parent-1", Name: "Department"}},
	}}}
	adapter := resourceproject.NewAdapter(client)
	project, err := adapter.NormalizeMutationProject(context.Background(), tableauproject.Project{LUID: "project-1", Name: "Operations", ParentLUID: "parent-1", Description: "Direct operations"})
	if err != nil {
		t.Fatal(err)
	}
	if project.LUID != "project-1" || project.Path != "Department/Operations" || project.Description != "Direct operations" {
		t.Fatalf("project = %#v", project)
	}
	root, err := adapter.NormalizeMutationProject(context.Background(), tableauproject.Project{LUID: "root-new", Name: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	if root.Path != "Root" {
		t.Fatalf("root = %#v", root)
	}
}
