package datasource_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type client struct {
	metadata     tableaudatasource.Datasource
	download     tableaudatasource.Download
	content      string
	getErr       error
	downloadErr  error
	calls        []string
	pages        map[int]tableaudatasource.Page
	pageSequence []tableaudatasource.Page
	listInputs   []tableaudatasource.ListRequest
	listErr      error
}

func (c *client) List(_ context.Context, input tableaudatasource.ListRequest) (tableaudatasource.Page, error) {
	c.calls = append(c.calls, "list:"+input.Name)
	c.listInputs = append(c.listInputs, input)
	if c.listErr != nil {
		return tableaudatasource.Page{}, c.listErr
	}
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

type filterRejectingClient struct {
	client
}

func (c *filterRejectingClient) List(ctx context.Context, input tableaudatasource.ListRequest) (tableaudatasource.Page, error) {
	if strings.ContainsAny(input.Name, ",&") {
		return tableaudatasource.Page{}, errors.New("Tableau rejected the datasource name filter")
	}
	return c.client.List(ctx, input)
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

func TestAdapterResolvesDatasourceContentURLsToAuthoritativeRecordsInOneBatch(t *testing.T) {
	page := tableaudatasource.Page{Number: 1, Size: 1000, Total: 2, Items: []tableaudatasource.Datasource{
		{LUID: "ds-b", Name: "Beta", ContentURL: "beta", ProjectLUID: "project-1", ProjectName: "Ops"},
		{LUID: "ds-a", Name: "Sales & Margin", ContentURL: "alpha", ProjectLUID: "project-1", ProjectName: "Ops", TableauRequestID: "request-1"},
	}}
	c := &client{pageSequence: []tableaudatasource.Page{page}}
	resolved, err := resourcedatasource.NewAdapter(c).ResolveContentURLs(t.Context(), []string{"beta", "alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 || resolved["alpha"].LUID != "ds-a" || resolved["alpha"].Name != "Sales & Margin" || resolved["alpha"].RequestID != "request-1" || resolved["beta"].LUID != "ds-b" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if len(c.listInputs) != 1 || !reflect.DeepEqual(c.listInputs[0].ContentURLs, []string{"alpha", "beta"}) {
		t.Fatalf("requests = %#v", c.listInputs)
	}
}

func TestAdapterFailsClosedWhenDatasourceContentURLResolutionIsIncompleteOrAmbiguous(t *testing.T) {
	alpha := tableaudatasource.Datasource{LUID: "ds-a", Name: "Alpha", ContentURL: "alpha", ProjectLUID: "project-1", ProjectName: "Ops"}
	for name, test := range map[string]struct {
		items []tableaudatasource.Datasource
		want  string
	}{
		"missing":    {items: []tableaudatasource.Datasource{alpha}, want: `"missing" did not resolve`},
		"unexpected": {items: []tableaudatasource.Datasource{alpha, {LUID: "ds-x", Name: "X", ContentURL: "x", ProjectLUID: "project-1", ProjectName: "Ops"}}, want: "unexpected identity"},
		"ambiguous":  {items: []tableaudatasource.Datasource{alpha, {LUID: "ds-a2", Name: "Alpha", ContentURL: "alpha", ProjectLUID: "project-2", ProjectName: "Ops"}}, want: "resolved ambiguously"},
		"incomplete": {items: []tableaudatasource.Datasource{{LUID: "ds-a", ContentURL: "alpha", ProjectLUID: "project-1", ProjectName: "Ops"}}, want: "authoritative LUID, name"},
	} {
		t.Run(name, func(t *testing.T) {
			c := &client{pageSequence: []tableaudatasource.Page{{Number: 1, Size: 1000, Total: len(test.items), Items: test.items}}}
			if _, err := resourcedatasource.NewAdapter(c).ResolveContentURLs(t.Context(), []string{"alpha", "missing"}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAdapterListsExactDatasourceProjectAcrossCompletePages(t *testing.T) {
	c := &client{pageSequence: []tableaudatasource.Page{
		{Number: 1, Size: 1, Total: 3, Items: []tableaudatasource.Datasource{{LUID: "ds-other", Name: "Published Source", ProjectLUID: "project-other", ProjectName: "Publish Project"}}},
		{Number: 2, Size: 1, Total: 3, Items: []tableaudatasource.Datasource{{LUID: "ds-target", Name: "Published Source", ProjectLUID: "project-1", ProjectName: "Publish Project"}}},
		{Number: 3, Size: 1, Total: 3, Items: []tableaudatasource.Datasource{{LUID: "ds-target-name", Name: "Another Source", ProjectLUID: "project-1", ProjectName: "Publish Project"}}},
	}}
	page, err := resourcedatasource.NewAdapter(c).ListDatasources(t.Context(), tableaudatasource.ListRequest{PageNumber: 1, PageSize: 1, Name: "Published Source", ProjectLUID: "project-1", ProjectName: "Publish Project"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 1 || page.Size != 1 || page.Total != 1 || len(page.Items) != 1 || page.Items[0].LUID != "ds-target" {
		t.Fatalf("page = %#v", page)
	}
	if len(c.listInputs) != 3 || c.listInputs[0].PageSize != 1000 || c.listInputs[0].ProjectLUID != "project-1" || c.listInputs[0].Name != "Published Source" {
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

func TestAdapterResolvesDatasourceByProjectLUID(t *testing.T) {
	c := &client{pages: map[int]tableaudatasource.Page{1: {
		Number: 1, Size: 2, Total: 2,
		Items: []tableaudatasource.Datasource{
			{LUID: "ds-other", Name: "Sales", ProjectLUID: "project-2", ProjectName: "Other"},
			{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops"},
		},
	}}}
	item, err := resourcedatasource.NewAdapterWithProjectResolver(c, projectPaths{"project-1": "Department/Ops", "project-2": "Other"}).ResolveDatasource(context.Background(), identity.Selector{Name: "Sales", ProjectLUID: "project-1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != "ds-1" || item.ProjectPath != "Department/Ops" || len(c.listInputs) != 1 || c.listInputs[0].ProjectLUID != "project-1" {
		t.Fatalf("item = %#v, inputs = %#v", item, c.listInputs)
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
	if len(c.listInputs) != 1 || c.listInputs[0].Name != "Sales" {
		t.Fatalf("list inputs = %#v", c.listInputs)
	}
}

func TestAdapterFindsExactDatasourceCollisionWhenNameCannotUseTableauFilterGrammar(t *testing.T) {
	c := &filterRejectingClient{client: client{pages: map[int]tableaudatasource.Page{
		1: {Number: 1, Size: 2, Total: 3, Items: []tableaudatasource.Datasource{
			{LUID: "ds-other", Name: "Other", ProjectLUID: "project-1", ProjectName: "Ops"},
			{LUID: "ds-wrong-project", Name: "Revenue & Profit, Daily", ProjectLUID: "project-2", ProjectName: "Other"},
		}},
		2: {Number: 2, Size: 2, Total: 3, Items: []tableaudatasource.Datasource{
			{LUID: "ds-match", Name: "Revenue & Profit, Daily", ProjectLUID: "project-1", ProjectName: "Ops"},
		}},
	}}}

	items, err := resourcedatasource.NewAdapter(c).FindDatasources(context.Background(), "Revenue & Profit, Daily", "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].LUID != "ds-match" {
		t.Fatalf("items = %#v", items)
	}
	if len(c.listInputs) != 2 || c.listInputs[0].Name != "" || c.listInputs[1].Name != "" {
		t.Fatalf("list inputs = %#v", c.listInputs)
	}
}

func (c *client) Download(_ context.Context, luid string, includeExtract *bool, destination io.Writer) (tableaudatasource.Download, error) {
	c.calls = append(c.calls, "download:"+luid)
	if includeExtract != nil {
		panic("dependency acquisition must preserve the server's native extract behavior")
	}
	if _, err := io.WriteString(destination, c.content); err != nil {
		return tableaudatasource.Download{}, err
	}
	return c.download, c.downloadErr
}

func TestAdapterDownloadsOneAuthoritativeDatasourceArtifact(t *testing.T) {
	c := &client{
		metadata: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Shared"},
		download: tableaudatasource.Download{Filename: "Sales.tdsx", TableauRequestID: "request-1"},
		content:  "native",
	}

	var content bytes.Buffer
	result, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1", &content)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.calls, []string{"get:ds-1", "download:ds-1"}) {
		t.Fatalf("calls = %v", c.calls)
	}
	if result.LUID != "ds-1" || result.Name != "Sales" || result.ProjectLUID != "project-1" || result.ProjectPath != "Shared" || result.Filename != "Sales.tdsx" || content.String() != "native" || result.TableauRequestID != "request-1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAdapterStopsBeforeDownloadWhenAuthoritativeReadFails(t *testing.T) {
	c := &client{getErr: errors.New("not found")}
	_, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1", io.Discard)
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
		_, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1", io.Discard)
		if err == nil || !reflect.DeepEqual(c.calls, []string{"get:ds-1"}) {
			t.Fatalf("metadata = %#v, error = %v, calls = %v", metadata, err, c.calls)
		}
	}
}

func TestAdapterFindsDatasourcesByExactContentURLsInBoundedBatches(t *testing.T) {
	contentURLs := make([]string, 0, 27)
	for index := range 26 {
		contentURLs = append(contentURLs, fmt.Sprintf("Source%02d", index))
	}
	contentURLs = append(contentURLs, "Source00")
	page := tableaudatasource.Page{Number: 1, Size: 1000, Total: 1, Items: []tableaudatasource.Datasource{{LUID: "ds-1", Name: "Source", ContentURL: "Source00", ProjectLUID: "project-1", ProjectName: "Ops"}}}
	c := &client{pageSequence: []tableaudatasource.Page{page, {Number: 1, Size: 1000}}}
	items, err := resourcedatasource.NewAdapter(c).FindDatasourcesByContentURL(context.Background(), contentURLs)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].LUID != "ds-1" || items[0].ProjectLUID != "project-1" {
		t.Fatalf("items = %#v", items)
	}
	if len(c.listInputs) != 2 || len(c.listInputs[0].ContentURLs) != 25 || !reflect.DeepEqual(c.listInputs[1].ContentURLs, []string{"Source25"}) || c.listInputs[0].PageNumber != 1 || c.listInputs[0].PageSize != 1000 {
		t.Fatalf("requests = %#v", c.listInputs)
	}
}

func TestAdapterFindsDatasourcesByExactNamesAndRejectsAnOverfullPage(t *testing.T) {
	item := tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales & Margin", ContentURL: "SalesMargin_1", ProjectLUID: "project-1", ProjectName: "Ops"}
	c := &client{pageSequence: []tableaudatasource.Page{{Number: 1, Size: 1000, Total: 1, Items: []tableaudatasource.Datasource{item}}}}
	items, err := resourcedatasource.NewAdapter(c).FindDatasourcesByName(context.Background(), []string{"Sales & Margin"})
	if err != nil || len(items) != 1 || !reflect.DeepEqual(c.listInputs[0].Names, []string{"Sales & Margin"}) {
		t.Fatalf("items = %#v err = %v requests = %#v", items, err, c.listInputs)
	}
	c = &client{pageSequence: []tableaudatasource.Page{{Number: 1, Size: 1000, Total: 1001, Items: []tableaudatasource.Datasource{item}}}}
	if _, err := resourcedatasource.NewAdapter(c).FindDatasourcesByName(context.Background(), []string{"Sales & Margin"}); err == nil || len(c.listInputs) != 1 {
		t.Fatalf("overfull page err = %v requests = %d", err, len(c.listInputs))
	}
}
