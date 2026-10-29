package app

import (
	"context"
	"encoding/json"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"reflect"
	"testing"
	"time"

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
	request := datasourceops.ListPageRequest{PageNumber: 1, PageSize: 1, Name: "Sales", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	page, err := reader.ListDatasources(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest := tableaudatasource.ListRequest{PageNumber: 1, PageSize: 1000, Name: "Sales", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	if !reflect.DeepEqual(client.listInput, wantRequest) || page.RequestID != "request-1" || len(page.Datasources) != 1 || page.Datasources[0].Description != "Sales data" || page.Datasources[0].Size == nil || *page.Datasources[0].Size != 42 {
		t.Fatalf("request = %#v, page = %#v", client.listInput, page)
	}
}

func TestDatasourceInspectProjectsExactResourceAndRequestID(t *testing.T) {
	client := &datasourceInventoryClient{item: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Description: "Sales data", Tags: []string{"daily"}, TableauRequestID: "request-1"}}
	resolver := resourcedatasource.NewAdapterWithProjectResolver(client, datasourceInventoryProjects{"p-1": "Department/Ops"})
	output, err := datasourceops.Inspect(t.Context(), resolver, datasourceops.InspectInput{Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(output.Datasource, datasourceops.InspectDatasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectPath: "Department/Ops", Type: "hyper", Description: "Sales data", Tags: []string{"daily"}, RequestID: "request-1"}) {
		t.Fatalf("item = %#v", output.Datasource)
	}
}

func TestDatasourceInspectCachePayloadPreservesSchema(t *testing.T) {
	item := datasourceops.InspectDatasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", Upstream: &datasourceops.Upstream{Status: "unavailable", Help: "inspect metadata"}, HasExtracts: new(false), Tags: []string{"daily"}, RequestID: "private-request"}
	entry, err := resourceEntry("dev", "sandbox", "datasource", item.LUID, item.Name, item.ProjectPath, item.OwnerLUID, "detail", time.Time{}, item)
	want := `{"upstream":{"status":"unavailable","databases":null,"tables":null,"complete":false,"help":"inspect metadata"},"luid":"ds-1","name":"Sales","project_luid":"p-1","project_path":"","has_extracts":false,"tags":["daily"]}`
	if err != nil || string(entry.Payload) != want || entry.ProjectLUID != "p-1" {
		t.Fatalf("entry=%+v payload=%s err=%v", entry, entry.Payload, err)
	}
	var cached datasourceops.Record
	if err := json.Unmarshal(entry.Payload, &cached); err != nil {
		t.Fatal(err)
	}
	if cached.Upstream == nil || cached.Upstream.Help != item.Upstream.Help || cached.HasExtracts == nil || *cached.HasExtracts || cached.RequestID != "" || cached.ProjectLUID != "p-1" {
		t.Fatalf("cached=%+v", cached)
	}
}
