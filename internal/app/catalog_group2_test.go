package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	catalogget "github.com/ahillspace/tadx/actions/catalog/get"
	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	catalogstatus "github.com/ahillspace/tadx/actions/catalog/status"
	corecatalog "github.com/ahillspace/tadx/internal/catalog"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type projectPages struct {
	pages map[int]resourceproject.Page
	errAt int
	calls []int
}

func (p *projectPages) ListProjects(_ context.Context, input resourceproject.ListRequest) (resourceproject.Page, error) {
	p.calls = append(p.calls, input.PageNumber)
	if input.PageNumber == p.errAt {
		return resourceproject.Page{}, errors.New("project page failed")
	}
	return p.pages[input.PageNumber], nil
}

type workbookPages struct {
	pages map[int]resourceworkbook.Page
	calls []int
}

func (p *workbookPages) ListWorkbooks(_ context.Context, input tableauworkbook.ListRequest) (resourceworkbook.Page, error) {
	p.calls = append(p.calls, input.PageNumber)
	return p.pages[input.PageNumber], nil
}

type datasourcePages struct {
	pages map[int]resourcedatasource.Page
	calls []int
}

func (p *datasourcePages) ListDatasources(_ context.Context, input tableaudatasource.ListRequest) (resourcedatasource.Page, error) {
	p.calls = append(p.calls, input.PageNumber)
	return p.pages[input.PageNumber], nil
}

type flowPages struct {
	pages map[int]resourceflow.Page
	calls []int
	errAt int
}

func (p *flowPages) ListFlows(_ context.Context, input tableauflow.ListRequest) (resourceflow.Page, error) {
	p.calls = append(p.calls, input.PageNumber)
	if input.PageNumber == p.errAt {
		return resourceflow.Page{}, errors.New("flow page failed")
	}
	return p.pages[input.PageNumber], nil
}

func TestCatalogInventoryExhaustsAdmittedPagesAndNormalizesProjectPaths(t *testing.T) {
	projects := &projectPages{pages: map[int]resourceproject.Page{
		1: {Number: 1, Size: 1, Total: 2, Items: []resourceproject.Project{{LUID: "project-root", Name: "Department", OwnerLUID: "user-1"}}},
		2: {Number: 2, Size: 1, Total: 2, Items: []resourceproject.Project{{LUID: "project-ops", Name: "Operations", ParentLUID: "project-root", OwnerLUID: "user-2"}}},
	}}
	workbooks := &workbookPages{pages: map[int]resourceworkbook.Page{1: {Number: 1, Size: 1, Total: 1, Items: []resourceworkbook.Workbook{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-ops", OwnerLUID: "user-3"}}}}}
	datasources := &datasourcePages{pages: map[int]resourcedatasource.Page{1: {Number: 1, Size: 1, Total: 1, Items: []resourcedatasource.Datasource{{LUID: "ds-1", Name: "Warehouse", ProjectLUID: "project-ops", OwnerLUID: "user-4"}}}}}
	flows := &flowPages{pages: map[int]resourceflow.Page{1: {Number: 1, Size: 1, Total: 1, Items: []resourceflow.Flow{{LUID: "flow-1", Name: "Prepare", ProjectLUID: "project-ops", OwnerLUID: "user-5"}}}}}
	generatedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	inventory := catalogInventory{projects: projects, workbooks: workbooks, datasources: datasources, flows: flows, now: func() time.Time { return generatedAt }}

	snapshot, err := inventory.Read(context.Background(), catalogrefresh.Input{Environment: "production", Site: "marketing", SiteResolved: true, Scopes: []string{"projects", "workbooks", "datasources", "flows"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projects.calls, []int{1, 2}) || !reflect.DeepEqual(workbooks.calls, []int{1}) || !reflect.DeepEqual(datasources.calls, []int{1}) || !reflect.DeepEqual(flows.calls, []int{1}) {
		t.Fatalf("page calls: projects=%v workbooks=%v datasources=%v flows=%v", projects.calls, workbooks.calls, datasources.calls, flows.calls)
	}
	want := []catalogrefresh.Record{
		{LUID: "ds-1", Kind: "datasource", Name: "Warehouse", ProjectPath: "Department/Operations", Owner: "user-4"},
		{LUID: "flow-1", Kind: "flow", Name: "Prepare", ProjectPath: "Department/Operations", Owner: "user-5"},
		{LUID: "project-root", Kind: "project", Name: "Department", ProjectPath: "Department", Owner: "user-1"},
		{LUID: "project-ops", Kind: "project", Name: "Operations", ProjectPath: "Department/Operations", Owner: "user-2"},
		{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Department/Operations", Owner: "user-3"},
	}
	if snapshot.GeneratedAt != generatedAt || snapshot.Source != "tableau-rest" || !reflect.DeepEqual(snapshot.Records, want) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestCatalogInventoryStopsOnAnyPageFailureWithoutPartialSnapshot(t *testing.T) {
	projects := &projectPages{pages: map[int]resourceproject.Page{1: {Number: 1, Size: 1, Total: 1, Items: []resourceproject.Project{{LUID: "project-1", Name: "Ops"}}}}}
	flows := &flowPages{pages: map[int]resourceflow.Page{1: {Number: 1, Size: 1, Total: 2, Items: []resourceflow.Flow{{LUID: "flow-1", Name: "One", ProjectLUID: "project-1"}}}}, errAt: 2}
	inventory := catalogInventory{projects: projects, flows: flows, now: time.Now}
	snapshot, err := inventory.Read(context.Background(), catalogrefresh.Input{Scopes: []string{"flows"}})
	if err == nil || snapshot.Records != nil || !reflect.DeepEqual(flows.calls, []int{1, 2}) {
		t.Fatalf("snapshot=%#v error=%v calls=%v", snapshot, err, flows.calls)
	}
}

func TestCatalogRefreshDoesNotPublishWhenAnyInventoryPageFails(t *testing.T) {
	root := t.TempDir()
	projects := &projectPages{pages: map[int]resourceproject.Page{1: {Number: 1, Size: 1, Total: 1, Items: []resourceproject.Project{{LUID: "project-1", Name: "Ops"}}}}}
	flows := &flowPages{pages: map[int]resourceflow.Page{1: {Number: 1, Size: 1, Total: 2, Items: []resourceflow.Flow{{LUID: "flow-1", Name: "One", ProjectLUID: "project-1"}}}}, errAt: 2}
	inventory := catalogInventory{projects: projects, flows: flows, now: time.Now}
	action := catalogrefresh.New(inventory, catalogGenerationWriter{store: corecatalog.NewFileStore(root, time.Now)})
	_, err := action.Execute(context.Background(), catalogrefresh.Input{Environment: "production", Site: "marketing", SiteResolved: true, Scopes: []string{"flows"}})
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	if _, statErr := os.Stat(filepath.Join(root, "catalog", "production.json")); !os.IsNotExist(statErr) {
		t.Fatalf("incomplete generation was published: %v", statErr)
	}
}

func TestCatalogStoreBridgesRoundTripWithoutAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := corecatalog.NewFileStore(root, func() time.Time { return now })
	writer := catalogGenerationWriter{store: store}
	written, err := writer.Replace(context.Background(), catalogrefresh.Generation{Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Source: "tableau-rest", Scopes: []string{"workbooks"}, Records: []catalogrefresh.Record{{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops", Owner: "user-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(written.Path) || written.Path != "catalog/production.json" {
		t.Fatalf("write path = %q", written.Path)
	}
	got, err := (catalogStoreGetter{store: store}).Get(context.Background(), catalogget.Input{Environment: "production", Site: "marketing", SiteResolved: true, LUID: "wb-1"})
	if err != nil || got.Item.LUID != "wb-1" || got.Generation.ID != written.GenerationID {
		t.Fatalf("get = %#v, error = %v", got, err)
	}
	status, err := (catalogStoreStatuser{store: store}).Status(context.Background(), catalogstatus.Input{Environment: "production", Site: "marketing", SiteResolved: true})
	if err != nil || filepath.IsAbs(status.Path) || status.Path != written.Path || status.Records != 1 {
		t.Fatalf("status = %#v, error = %v", status, err)
	}
}
