package datasource_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type client struct {
	metadata     tableaudatasource.Datasource
	download     tableaudatasource.Download
	getErr       error
	downloadErr  error
	calls        []string
	pages        map[int]tableaudatasource.Page
	pageSequence []tableaudatasource.Page
	listInputs   []tableaudatasource.ListRequest
}

func (c *client) List(_ context.Context, input tableaudatasource.ListRequest) (tableaudatasource.Page, error) {
	c.calls = append(c.calls, "list:"+input.Name)
	c.listInputs = append(c.listInputs, input)
	if len(c.pageSequence) > 0 {
		index := len(c.listInputs) - 1
		if index >= len(c.pageSequence) {
			index = len(c.pageSequence) - 1
		}
		return c.pageSequence[index], nil
	}
	return c.pages[input.PageNumber], nil
}

func (c *client) Get(_ context.Context, luid string) (tableaudatasource.Datasource, error) {
	c.calls = append(c.calls, "get:"+luid)
	return c.metadata, c.getErr
}

type projectPaths map[string]string

func (p projectPaths) ResolveProjectPath(_ context.Context, luid string) (string, error) {
	path, ok := p[luid]
	if !ok {
		return "", errors.New("project not found")
	}
	return path, nil
}

func TestAdapterResolvesDatasourceByAuthoritativeLUID(t *testing.T) {
	c := &client{metadata: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops", TableauRequestID: "get-request"}}
	item, err := resourcedatasource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "Department/Ops"}).ResolveDatasource(context.Background(), identity.Selector{LUID: "ds-1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != "ds-1" || item.Name != "Sales" || item.ProjectLUID != "project-1" || item.ProjectPath != "Department/Ops" || item.RequestID != "get-request" {
		t.Fatalf("item = %#v", item)
	}
	if !reflect.DeepEqual(c.calls, []string{"get:ds-1"}) {
		t.Fatalf("calls = %v", c.calls)
	}
}

func TestAdapterListsOneRichDatasourcePage(t *testing.T) {
	size := int64(42)
	c := &client{pages: map[int]tableaudatasource.Page{1: {
		Number: 1, Size: 1, Total: 1, TableauRequestID: "request-1",
		Items: []tableaudatasource.Datasource{{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops", Description: "Sales data", OwnerLUID: "user-1", Type: "hyper", Size: &size, Tags: []string{"daily"}}},
	}}}
	request := tableaudatasource.ListRequest{PageNumber: 1, PageSize: 1, ProjectName: "Ops", OwnerName: "owner", Tag: "daily"}
	page, err := resourcedatasource.NewAdapter(c).ListDatasources(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if page.RequestID != "request-1" || len(page.Items) != 1 || page.Items[0].Description != "Sales data" || page.Items[0].Size == nil || *page.Items[0].Size != 42 || len(page.Items[0].Tags) != 1 {
		t.Fatalf("page = %#v", page)
	}
	if !reflect.DeepEqual(c.listInputs, []tableaudatasource.ListRequest{request}) {
		t.Fatalf("inputs = %#v", c.listInputs)
	}
}

func TestAdapterResolvesDatasourceByExactNameAndCanonicalProjectPath(t *testing.T) {
	c := &client{pages: map[int]tableaudatasource.Page{
		1: {Number: 1, Size: 2, Total: 3, TableauRequestID: "list-request-1", Items: []tableaudatasource.Datasource{
			{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"},
			{LUID: "ds-2", Name: "Sales", ProjectLUID: "project-2", ProjectName: "Ops"},
		}},
		2: {Number: 2, Size: 2, Total: 3, TableauRequestID: "list-request-2", Items: []tableaudatasource.Datasource{
			{LUID: "ds-3", Name: "Other", ProjectLUID: "project-1", ProjectName: "Ops"},
		}},
	}}
	item, err := resourcedatasource.NewAdapterWithProjectResolver(c, projectPaths{
		"project-1": "Department/Ops",
		"project-2": "Other/Ops",
	}).ResolveDatasource(context.Background(), identity.Selector{Name: "Sales", ProjectPath: "Department/Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != "ds-1" || item.ProjectPath != "Department/Ops" || item.RequestID != "list-request-2" {
		t.Fatalf("item = %#v", item)
	}
}

func TestAdapterRejectsAmbiguousDatasourceSelectorDeterministically(t *testing.T) {
	c := &client{pages: map[int]tableaudatasource.Page{1: {
		Number: 1, Size: 2, Total: 2,
		Items: []tableaudatasource.Datasource{
			{LUID: "ds-z", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"},
			{LUID: "ds-a", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"},
		},
	}}}
	_, err := resourcedatasource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "Department/Ops"}).ResolveDatasource(context.Background(), identity.Selector{Name: "Sales", ProjectPath: "Department/Ops"})
	if err == nil || !strings.Contains(err.Error(), "[ds-a, ds-z]") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdapterRejectsInvalidDatasourceSelector(t *testing.T) {
	c := &client{}
	adapter := resourcedatasource.NewAdapterWithProjectResolver(c, projectPaths{})
	for _, selector := range []identity.Selector{
		{},
		{Name: "Sales"},
		{ProjectPath: "Department/Ops"},
	} {
		if _, err := adapter.ResolveDatasource(context.Background(), selector); err == nil {
			t.Fatalf("selector %#v succeeded", selector)
		}
	}
}

func TestAdapterRejectsConflictingDatasourceRowsAcrossPages(t *testing.T) {
	c := &client{pages: map[int]tableaudatasource.Page{
		1: {Number: 1, Size: 1, Total: 2, Items: []tableaudatasource.Datasource{{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"}}},
		2: {Number: 2, Size: 1, Total: 2, Items: []tableaudatasource.Datasource{{LUID: "ds-1", Name: "Changed", ProjectLUID: "project-1", ProjectName: "Ops"}}},
	}}
	_, err := resourcedatasource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "Department/Ops"}).ResolveDatasource(context.Background(), identity.Selector{Name: "Sales", ProjectPath: "Department/Ops"})
	if err == nil || !strings.Contains(err.Error(), `conflicting records for LUID "ds-1"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestAdapterRejectsChangingDatasourcePagination(t *testing.T) {
	c := &client{pages: map[int]tableaudatasource.Page{
		1: {Number: 1, Size: 1, Total: 2, Items: []tableaudatasource.Datasource{{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"}}},
		2: {Number: 2, Size: 1, Total: 3, Items: []tableaudatasource.Datasource{{LUID: "ds-2", Name: "Sales", ProjectLUID: "project-2", ProjectName: "Ops"}}},
	}}
	_, err := resourcedatasource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "One", "project-2": "Two"}).ResolveDatasource(context.Background(), identity.Selector{Name: "Sales", ProjectPath: "One"})
	if err == nil || !strings.Contains(err.Error(), "pagination total changed") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdapterFindsExactDatasourceCollisionsByProjectLUID(t *testing.T) {
	c := &client{pages: map[int]tableaudatasource.Page{
		1: {Number: 1, Size: 3, Total: 3, Items: []tableaudatasource.Datasource{
			{LUID: "ds-z", Name: "Sales", ProjectLUID: "other", ProjectName: "Ops"},
			{LUID: "ds-b", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"},
			{LUID: "ds-a", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"},
		}},
	}}
	items, err := resourcedatasource.NewAdapter(c).FindDatasources(context.Background(), "Sales", "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].LUID != "ds-a" || items[1].LUID != "ds-b" {
		t.Fatalf("items = %#v", items)
	}
}

func TestAdapterWaitsForCompletedDatasourceToBecomeAuthoritativelyVisible(t *testing.T) {
	c := &client{pageSequence: []tableaudatasource.Page{
		{Number: 1, Size: 1000, Total: 0},
		{Number: 1, Size: 1000, Total: 1, TableauRequestID: "resolve-request", Items: []tableaudatasource.Datasource{{LUID: "ds-new", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Analytics"}}},
	}}
	adapter := resourcedatasource.NewAdapter(c)
	adapter.SetCompletionResolvePolicy(time.Millisecond, 100*time.Millisecond)
	item, err := adapter.ResolvePublishedDatasource(context.Background(), "Sales", "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != "ds-new" || item.Name != "Sales" || item.ProjectLUID != "project-1" || len(c.listInputs) != 2 {
		t.Fatalf("item = %#v, calls = %#v", item, c.listInputs)
	}
}

func TestAdapterBoundsMissingCompletedDatasourceResolution(t *testing.T) {
	c := &client{pageSequence: []tableaudatasource.Page{{Number: 1, Size: 1000, Total: 0}}}
	adapter := resourcedatasource.NewAdapter(c)
	adapter.SetCompletionResolvePolicy(time.Millisecond, 10*time.Millisecond)
	_, err := adapter.ResolvePublishedDatasource(context.Background(), "Sales", "project-1")
	if err == nil || !strings.Contains(err.Error(), "resolution deadline") || len(c.listInputs) < 2 {
		t.Fatalf("error = %v, calls = %d", err, len(c.listInputs))
	}
}

func TestAdapterRejectsAmbiguousCompletedDatasourceResolution(t *testing.T) {
	c := &client{pageSequence: []tableaudatasource.Page{{Number: 1, Size: 1000, Total: 2, Items: []tableaudatasource.Datasource{
		{LUID: "ds-b", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Analytics"},
		{LUID: "ds-a", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Analytics"},
	}}}}
	adapter := resourcedatasource.NewAdapter(c)
	adapter.SetCompletionResolvePolicy(time.Millisecond, 100*time.Millisecond)
	_, err := adapter.ResolvePublishedDatasource(context.Background(), "Sales", "project-1")
	if err == nil || !strings.Contains(err.Error(), "ambiguous: [ds-a, ds-b]") || len(c.listInputs) != 1 {
		t.Fatalf("error = %v, calls = %d", err, len(c.listInputs))
	}
}

func (c *client) Download(_ context.Context, luid string, includeExtract *bool) (tableaudatasource.Download, error) {
	c.calls = append(c.calls, "download:"+luid)
	if includeExtract != nil {
		panic("dependency acquisition must preserve the server's native extract behavior")
	}
	return c.download, c.downloadErr
}

func TestAdapterDownloadsOneAuthoritativeDatasourceArtifact(t *testing.T) {
	c := &client{
		metadata: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Shared"},
		download: tableaudatasource.Download{Filename: "Sales.tdsx", Content: []byte("native"), TableauRequestID: "request-1"},
	}

	result, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.calls, []string{"get:ds-1", "download:ds-1"}) {
		t.Fatalf("calls = %v", c.calls)
	}
	if result.LUID != "ds-1" || result.Name != "Sales" || result.ProjectLUID != "project-1" || result.ProjectPath != "Shared" || result.Filename != "Sales.tdsx" || string(result.Content) != "native" || result.TableauRequestID != "request-1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAdapterStopsBeforeDownloadWhenAuthoritativeReadFails(t *testing.T) {
	c := &client{getErr: errors.New("not found")}
	_, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1")
	if err == nil || !reflect.DeepEqual(c.calls, []string{"get:ds-1"}) {
		t.Fatalf("error = %v, calls = %v", err, c.calls)
	}
}

func TestAdapterRejectsIncompleteDatasourceIdentity(t *testing.T) {
	for _, metadata := range []tableaudatasource.Datasource{
		{Name: "Sales"},
		{LUID: "ds-other", Name: "Sales"},
		{LUID: "ds-1"},
		{LUID: "ds-1", Name: "Sales"},
	} {
		c := &client{metadata: metadata}
		_, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1")
		if err == nil || !reflect.DeepEqual(c.calls, []string{"get:ds-1"}) {
			t.Fatalf("metadata = %#v, error = %v, calls = %v", metadata, err, c.calls)
		}
	}
}
