package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	datasourceget "github.com/ahillspace/tadx/actions/datasource/inspect"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type datasourceInventoryClient struct {
	page      tableaudatasource.Page
	item      tableaudatasource.Datasource
	listInput tableaudatasource.ListRequest
}

func (c *datasourceInventoryClient) List(_ context.Context, input tableaudatasource.ListRequest) (tableaudatasource.Page, error) {
	c.listInput = input
	return c.page, nil
}

func (c *datasourceInventoryClient) Get(context.Context, string) (tableaudatasource.Datasource, error) {
	return c.item, nil
}

func (*datasourceInventoryClient) Download(context.Context, string, *bool) (tableaudatasource.Download, error) {
	return tableaudatasource.Download{}, errors.New("not used")
}

type datasourceInventoryProjects map[string]string

func (p datasourceInventoryProjects) ResolveProjectPath(_ context.Context, luid string) (string, error) {
	path, ok := p[luid]
	if !ok {
		return "", errors.New("project not found")
	}
	return path, nil
}

func TestDatasourceListReaderMapsActionRequestAndRichResourcePage(t *testing.T) {
	size := int64(42)
	client := &datasourceInventoryClient{page: tableaudatasource.Page{
		Number: 1, Size: 1, Total: 1, TableauRequestID: "request-1",
		Items: []tableaudatasource.Datasource{{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Description: "Sales data", Size: &size, Tags: []string{"daily"}}},
	}}
	reader := datasourceListReader{adapter: resourcedatasource.NewAdapter(client)}
	request := datasourcelist.PageRequest{PageNumber: 1, PageSize: 1, Name: "Sales", OwnerName: "owner", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	page, err := reader.ListDatasources(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest := tableaudatasource.ListRequest{PageNumber: 1, PageSize: 1, Name: "Sales", OwnerName: "owner", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	if !reflect.DeepEqual(client.listInput, wantRequest) || page.RequestID != "request-1" || len(page.Datasources) != 1 || page.Datasources[0].Description != "Sales data" || page.Datasources[0].Size == nil || *page.Datasources[0].Size != 42 {
		t.Fatalf("request = %#v, page = %#v", client.listInput, page)
	}
}

func TestDatasourceGetResolverMapsExactResourceAndRequestID(t *testing.T) {
	client := &datasourceInventoryClient{item: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Description: "Sales data", Tags: []string{"daily"}, TableauRequestID: "request-1"}}
	resolver := datasourceGetResolver{resourcedatasource.NewAdapterWithProjectResolver(client, datasourceInventoryProjects{"p-1": "Department/Ops"})}
	item, err := resolver.ResolveDatasource(context.Background(), identity.Selector{LUID: "ds-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(item, datasourceget.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectPath: "Department/Ops", Type: "hyper", Description: "Sales data", Tags: []string{"daily"}, RequestID: "request-1"}) {
		t.Fatalf("item = %#v", item)
	}
}
