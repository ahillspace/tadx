package datasource_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type datasourceReadProjects map[string]string

func (p datasourceReadProjects) ResolveProjectPath(_ context.Context, luid string) (string, error) {
	path, ok := p[luid]
	if !ok {
		return "", errors.New("project not found")
	}
	return path, nil
}

func (p datasourceReadProjects) ResolveProjectPaths(ctx context.Context, luids []string) (map[string]string, error) {
	paths := make(map[string]string, len(luids))
	for _, luid := range luids {
		path, err := p.ResolveProjectPath(ctx, luid)
		if err != nil {
			return nil, err
		}
		paths[luid] = path
	}
	return paths, nil
}

func TestReadPortsMapActionRequestAndRichPage(t *testing.T) {
	size := int64(42)
	native := &client{pages: map[int]tableaudatasource.Page{1: {
		Number: 1, Size: 1, Total: 1, TableauRequestID: "request-1",
		Items: []tableaudatasource.Datasource{{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Description: "Sales data", Size: &size, Tags: []string{"daily"}}},
	}}}
	ports := resourcedatasource.ReadPorts{Adapter: resourcedatasource.NewAdapter(native), Projects: datasourceReadProjects{"p-1": "Department/Ops"}}
	request := datasourceops.ListPageRequest{PageNumber: 1, PageSize: 1, Name: "Sales", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	page, err := ports.ListDatasources(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := tableaudatasource.ListRequest{PageNumber: 1, PageSize: 1000, Name: "Sales", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	if len(native.listInputs) != 1 || !reflect.DeepEqual(native.listInputs[0], want) || page.RequestID != "request-1" || len(page.Datasources) != 1 || page.Datasources[0].Description != "Sales data" || page.Datasources[0].ProjectPath != "Department/Ops" || page.Datasources[0].Size == nil || *page.Datasources[0].Size != 42 {
		t.Fatalf("request = %#v, page = %#v", native.listInputs, page)
	}
}

func TestReadPortsInspectProjectsExactResourceAndRequestID(t *testing.T) {
	native := &client{metadata: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Description: "Sales data", Tags: []string{"daily"}, TableauRequestID: "request-1"}}
	ports := resourcedatasource.ReadPorts{Adapter: resourcedatasource.NewAdapterWithProjectResolver(native, datasourceReadProjects{"p-1": "Department/Ops"})}
	item, err := ports.ResolveDatasource(t.Context(), identity.Selector{LUID: "ds-1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != "ds-1" || item.Name != "Sales" || item.ProjectLUID != "p-1" || item.ProjectPath != "Department/Ops" || item.Type != "hyper" || item.Description != "Sales data" || !reflect.DeepEqual(item.Tags, []string{"daily"}) || item.RequestID != "request-1" {
		t.Fatalf("item = %#v", item)
	}
}

func TestReadPortsInspectCachePayloadPreservesSchema(t *testing.T) {
	store := cache.NewStore(t.TempDir(), time.Now)
	item := datasourceops.InspectDatasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", Upstream: &datasourceops.Upstream{Status: "unavailable", Help: "inspect metadata"}, HasExtracts: new(false), Tags: []string{"daily"}, RequestID: "private-request"}
	ports := resourcedatasource.InventoryPorts{Store: func() *cache.Store { return store }}
	ports.PublishDatasourceInspect(t.Context(), datasourceops.InspectOutput{Environment: "dev", Site: "sandbox", Datasource: item}, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
	result, err := store.ReadResources(t.Context(), cache.ResourceQuery{Environment: "dev", Site: "sandbox", Kind: "datasource", LUID: "ds-1", Limit: 1, ExactlyOne: true})
	if err != nil || len(result.Entries) != 1 {
		t.Fatalf("cached result=%#v err=%v", result, err)
	}
	entry := result.Entries[0]
	want := `{"upstream":{"status":"unavailable","databases":null,"tables":null,"complete":false,"help":"inspect metadata"},"luid":"ds-1","name":"Sales","project_luid":"p-1","project_path":"","has_extracts":false,"tags":["daily"]}`
	if string(entry.Payload) != want || entry.ProjectLUID != "p-1" {
		t.Fatalf("entry=%+v payload=%s", entry, entry.Payload)
	}
	var cached datasourceops.Record
	if err := json.Unmarshal(entry.Payload, &cached); err != nil {
		t.Fatal(err)
	}
	if cached.Upstream == nil || cached.Upstream.Help != item.Upstream.Help || cached.HasExtracts == nil || *cached.HasExtracts || cached.RequestID != "" || cached.ProjectLUID != "p-1" {
		t.Fatalf("cached=%+v", cached)
	}
}
