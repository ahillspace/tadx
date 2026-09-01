package flow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

type flowClient struct {
	pages    map[int]tableauflow.Page
	byLUID   map[string]tableauflow.Flow
	download tableauflow.Download
}

func (c flowClient) List(_ context.Context, input tableauflow.ListRequest) (tableauflow.Page, error) {
	return c.pages[input.PageNumber], nil
}
func (c flowClient) Get(_ context.Context, luid string) (tableauflow.Flow, error) {
	item, exists := c.byLUID[luid]
	if !exists {
		return tableauflow.Flow{}, errors.New("not found")
	}
	return item, nil
}
func (c flowClient) Download(context.Context, string) (tableauflow.Download, error) {
	return c.download, nil
}

type paths map[string]string

func (p paths) ResolveProjectPath(_ context.Context, luid string) (string, error) {
	path, exists := p[luid]
	if !exists {
		return "", errors.New("project not found")
	}
	return path, nil
}

func TestAdapterResolvesExactNestedFlow(t *testing.T) {
	client := flowClient{pages: map[int]tableauflow.Page{1: {Number: 1, Size: 2, Total: 2, Items: []tableauflow.Flow{
		{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectName: "Ops"},
		{LUID: "f-2", Name: "Daily", ProjectLUID: "p-2", ProjectName: "Ops"},
	}}}}
	item, err := resourceflow.NewAdapter(client, paths{"p-1": "Department/Ops", "p-2": "Other/Ops"}).ResolveFlow(context.Background(), identity.Selector{Name: "Daily", ProjectPath: "Department/Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != "f-1" || item.ProjectPath != "Department/Ops" {
		t.Fatalf("flow = %#v", item)
	}
}

func TestAdapterRejectsIncompleteFlowIdentity(t *testing.T) {
	client := flowClient{byLUID: map[string]tableauflow.Flow{"f-1": {LUID: "f-1", Name: "Daily"}}}
	_, err := resourceflow.NewAdapter(client, paths{}).ResolveFlow(context.Background(), identity.Selector{LUID: "f-1"})
	if err == nil {
		t.Fatal("expected incomplete identity error")
	}
}

func TestAdapterPreservesNativeFlowBytes(t *testing.T) {
	client := flowClient{download: tableauflow.Download{Filename: "Daily.tflx", Content: []byte("native\x00flow"), TableauRequestID: "request-1"}}
	download, err := resourceflow.NewAdapter(client, paths{}).DownloadFlow(context.Background(), "f-1")
	if err != nil {
		t.Fatal(err)
	}
	if download.Filename != "Daily.tflx" || string(download.Content) != "native\x00flow" {
		t.Fatalf("download = %#v", download)
	}
}

func TestAdapterFindsProjectScopedCaseInsensitiveCollision(t *testing.T) {
	client := flowClient{pages: map[int]tableauflow.Page{1: {Number: 1, Size: 1000, Total: 2, Items: []tableauflow.Flow{
		{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1"},
		{LUID: "f-2", Name: "daily", ProjectLUID: "p-2"},
	}}}}
	items, err := resourceflow.NewAdapter(client, paths{"p-1": "One", "p-2": "Two"}).FindFlows(context.Background(), "DAILY", "p-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].LUID != "f-2" {
		t.Fatalf("items = %#v", items)
	}
}

func TestAdapterRejectsChangingFlowPagination(t *testing.T) {
	client := flowClient{pages: map[int]tableauflow.Page{
		1: {Number: 1, Size: 1, Total: 2, Items: []tableauflow.Flow{{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectName: "Ops"}}},
		2: {Number: 2, Size: 1, Total: 3, Items: []tableauflow.Flow{{LUID: "f-2", Name: "Daily", ProjectLUID: "p-2", ProjectName: "Ops"}}},
	}}
	_, err := resourceflow.NewAdapter(client, paths{"p-1": "One", "p-2": "Two"}).ResolveFlow(context.Background(), identity.Selector{Name: "Daily", ProjectPath: "One"})
	if err == nil || !strings.Contains(err.Error(), "pagination total changed") {
		t.Fatalf("error = %v", err)
	}
}
