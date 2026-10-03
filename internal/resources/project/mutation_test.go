package project_test

import (
	"context"
	"testing"

	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

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

type mutationClient struct {
	*projectClient
	result tableauproject.MutationResult
}

func (c *mutationClient) Create(context.Context, tableauproject.CreateRequest) (tableauproject.MutationResult, error) {
	return tableauproject.MutationResult{}, nil
}

func (c *mutationClient) Update(context.Context, tableauproject.UpdateRequest) (tableauproject.MutationResult, error) {
	return c.result, nil
}

func (c *mutationClient) Delete(context.Context, string) (tableauproject.DeleteResult, error) {
	return tableauproject.DeleteResult{}, nil
}

func TestMovePortNormalizesReturnedHierarchyPath(t *testing.T) {
	parent := "parent-2"
	client := &mutationClient{
		projectClient: &projectClient{pages: map[int]tableauproject.Page{1: {
			Number: 1, Size: 1000, Total: 1, Items: []tableauproject.Project{{LUID: parent, Name: "Department"}},
		}}},
		result: tableauproject.MutationResult{Status: "succeeded", Project: tableauproject.Project{LUID: "project-1", Name: "Operations", ParentLUID: parent}, TableauRequestID: "request-project"},
	}
	port := resourceproject.NewMovePort(resourceproject.NewAdapter(client), client)
	result, err := port.MoveProject(t.Context(), "project-1", &parent)
	if err != nil || result.Project.Path != "Department/Operations" || result.Project.ParentLUID != parent || result.TableauRequestID != "request-project" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMovePortReportsLiteralSlashPathUnavailable(t *testing.T) {
	parent := "parent-2"
	client := &mutationClient{
		projectClient: &projectClient{pages: map[int]tableauproject.Page{1: {
			Number: 1, Size: 1000, Total: 1, Items: []tableauproject.Project{{LUID: parent, Name: "Department"}},
		}}},
		result: tableauproject.MutationResult{Status: "succeeded", Project: tableauproject.Project{LUID: "project-1", Name: "Ops/Reports", ParentLUID: parent}, TableauRequestID: "request-project"},
	}
	port := resourceproject.NewMovePort(resourceproject.NewAdapter(client), client)
	result, err := port.MoveProject(t.Context(), "project-1", &parent)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Path != "" || result.Project.PathUnavailableReason != resourceproject.LiteralSlashPathUnavailable {
		t.Fatalf("result=%#v", result)
	}
}
