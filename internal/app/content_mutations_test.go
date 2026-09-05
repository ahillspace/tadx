package app

import (
	"context"
	"testing"

	datasourcemove "github.com/ahillspace/tadx/actions/datasource/move"
	datasourceupdate "github.com/ahillspace/tadx/actions/datasource/update"
	flowupdate "github.com/ahillspace/tadx/actions/flow/update"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	workbookmove "github.com/ahillspace/tadx/actions/workbook/move"
	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

var (
	_ workbookmove.Resolver     = workbookMutationAdapter{}
	_ workbookmove.Mover        = workbookMutationAdapter{}
	_ workbookupdate.Resolver   = workbookUpdateAdapter{}
	_ workbookupdate.Updater    = workbookUpdateAdapter{}
	_ datasourcemove.Resolver   = datasourceMutationAdapter{}
	_ datasourcemove.Mover      = datasourceMutationAdapter{}
	_ datasourceupdate.Resolver = datasourceUpdateAdapter{}
	_ datasourceupdate.Updater  = datasourceUpdateAdapter{}
	_ flowupdate.Resolver       = (*flowUpdateAdapter)(nil)
	_ flowupdate.Updater        = (*flowUpdateAdapter)(nil)
	_ projectmove.Resolver      = projectMoveAdapter{}
	_ projectmove.Mover         = projectMoveAdapter{}
)

type contentMutationWorkbookClient struct {
	result tableauworkbook.MutationResult
}

func (c *contentMutationWorkbookClient) Get(context.Context, string) (tableauworkbook.Workbook, error) {
	return tableauworkbook.Workbook{}, nil
}
func (c *contentMutationWorkbookClient) List(context.Context, int, int) (tableauworkbook.WorkbookPage, error) {
	return tableauworkbook.WorkbookPage{}, nil
}
func (c *contentMutationWorkbookClient) ListProjects(context.Context, int, int) (tableauworkbook.ProjectPage, error) {
	return tableauworkbook.ProjectPage{}, nil
}
func (c *contentMutationWorkbookClient) Download(context.Context, string, *bool) (tableauworkbook.Download, error) {
	return tableauworkbook.Download{}, nil
}
func (c *contentMutationWorkbookClient) Prepare(context.Context, tableauworkbook.PublishRequest) (*tableauworkbook.PreparedPublish, error) {
	return nil, nil
}
func (c *contentMutationWorkbookClient) Update(context.Context, tableauworkbook.UpdateRequest) (tableauworkbook.MutationResult, error) {
	return c.result, nil
}

type contentMutationDatasourceClient struct {
	result tableaudatasource.MutationResult
}

func (c *contentMutationDatasourceClient) Get(context.Context, string) (tableaudatasource.Datasource, error) {
	return tableaudatasource.Datasource{}, nil
}
func (c *contentMutationDatasourceClient) List(context.Context, tableaudatasource.ListRequest) (tableaudatasource.Page, error) {
	return tableaudatasource.Page{}, nil
}
func (c *contentMutationDatasourceClient) Download(context.Context, string, *bool) (tableaudatasource.Download, error) {
	return tableaudatasource.Download{}, nil
}
func (c *contentMutationDatasourceClient) Prepare(context.Context, tableaudatasource.PublishRequest) (tableaudatasource.PreparedPublish, error) {
	return nil, nil
}
func (c *contentMutationDatasourceClient) Delete(context.Context, string) (tableaudatasource.MutationResult, error) {
	return tableaudatasource.MutationResult{}, nil
}
func (c *contentMutationDatasourceClient) Update(context.Context, tableaudatasource.UpdateRequest) (tableaudatasource.MutationResult, error) {
	return c.result, nil
}

type contentMutationProjectPath struct{}

func (contentMutationProjectPath) ResolveProjectPath(context.Context, string) (string, error) {
	return "Operations", nil
}

type contentMutationFlowClient struct {
	flow   tableauflow.Flow
	result tableauflow.MutationResult
}

func (c *contentMutationFlowClient) List(context.Context, tableauflow.ListRequest) (tableauflow.Page, error) {
	return tableauflow.Page{}, nil
}
func (c *contentMutationFlowClient) Get(context.Context, string) (tableauflow.Flow, error) {
	return c.flow, nil
}
func (c *contentMutationFlowClient) Download(context.Context, string) (tableauflow.Download, error) {
	return tableauflow.Download{}, nil
}
func (c *contentMutationFlowClient) Prepare(context.Context, tableauflow.PublishRequest) (tableauflow.PreparedPublish, error) {
	return nil, nil
}
func (c *contentMutationFlowClient) Move(context.Context, string, string) (tableauflow.MutationResult, error) {
	return tableauflow.MutationResult{}, nil
}
func (c *contentMutationFlowClient) Delete(context.Context, string) (tableauflow.MutationResult, error) {
	return tableauflow.MutationResult{}, nil
}
func (c *contentMutationFlowClient) Update(context.Context, tableauflow.UpdateRequest) (tableauflow.MutationResult, error) {
	return c.result, nil
}

type contentMutationProjectClient struct {
	page   tableauproject.Page
	result tableauproject.MutationResult
}

func (c *contentMutationProjectClient) List(context.Context, tableauproject.ListRequest) (tableauproject.Page, error) {
	return c.page, nil
}
func (c *contentMutationProjectClient) Create(context.Context, tableauproject.CreateRequest) (tableauproject.MutationResult, error) {
	return tableauproject.MutationResult{}, nil
}
func (c *contentMutationProjectClient) Update(context.Context, tableauproject.UpdateRequest) (tableauproject.MutationResult, error) {
	return c.result, nil
}
func (c *contentMutationProjectClient) Delete(context.Context, string) (tableauproject.DeleteResult, error) {
	return tableauproject.DeleteResult{}, nil
}

func TestContentMutationAdaptersPreserveAuthoritativeResults(t *testing.T) {
	ctx := context.Background()
	workbookClient := &contentMutationWorkbookClient{result: tableauworkbook.MutationResult{Status: "succeeded", WorkbookLUID: "wb-1", WorkbookName: "Renamed", ProjectLUID: "project-2", OwnerLUID: "owner-2", TableauRequestID: "request-wb"}}
	workbooks := resourceworkbook.NewAdapter(workbookClient)
	projectLUID := "project-2"
	moveResult, err := (workbookMutationAdapter{workbooks: workbooks}).MoveWorkbook(ctx, "wb-1", projectLUID)
	if err != nil || moveResult.WorkbookLUID != "wb-1" || moveResult.ProjectLUID != projectLUID || moveResult.TableauRequestID != "request-wb" {
		t.Fatalf("workbook move=%#v err=%v", moveResult, err)
	}
	name, owner := "Renamed", "owner-2"
	updateResult, err := (workbookUpdateAdapter{workbooks: workbooks}).UpdateWorkbook(ctx, workbookupdate.Request{LUID: "wb-1", Name: &name, OwnerLUID: &owner})
	if err != nil || updateResult.WorkbookName != name || updateResult.OwnerLUID != owner {
		t.Fatalf("workbook update=%#v err=%v", updateResult, err)
	}

	datasourceClient := &contentMutationDatasourceClient{result: tableaudatasource.MutationResult{Status: "succeeded", DatasourceLUID: "ds-1", DatasourceName: "Renamed", ProjectLUID: "project-2", OwnerLUID: "owner-2", TableauRequestID: "request-ds"}}
	datasources := resourcedatasource.NewAdapter(datasourceClient)
	datasourceChanges := resourcedatasource.NewMutationAdapter(datasourceClient)
	datasourceMoveResult, err := (datasourceMutationAdapter{datasources: datasources, changes: datasourceChanges}).MoveDatasource(ctx, "ds-1", projectLUID)
	if err != nil || datasourceMoveResult.DatasourceLUID != "ds-1" || datasourceMoveResult.ProjectLUID != projectLUID || datasourceMoveResult.TableauRequestID != "request-ds" {
		t.Fatalf("datasource move=%#v err=%v", datasourceMoveResult, err)
	}
	datasourceUpdateResult, err := (datasourceUpdateAdapter{datasources: datasources, changes: datasourceChanges}).UpdateDatasource(ctx, datasourceupdate.Request{LUID: "ds-1", Name: &name, OwnerLUID: &owner})
	if err != nil || datasourceUpdateResult.DatasourceName != name || datasourceUpdateResult.OwnerLUID != owner {
		t.Fatalf("datasource update=%#v err=%v", datasourceUpdateResult, err)
	}
}

func TestFlowUpdateAdapterEnrichesOnlyFromResolvedAuthoritativeFlow(t *testing.T) {
	owner := "owner-2"
	client := &contentMutationFlowClient{
		flow:   tableauflow.Flow{LUID: "flow-1", Name: "Daily Prep", ProjectLUID: "project-1", ProjectName: "Operations", OwnerLUID: "owner-1"},
		result: tableauflow.MutationResult{Status: "succeeded", FlowLUID: "flow-1", OwnerLUID: owner, TableauRequestID: "request-flow"},
	}
	flows := resourceflow.NewAdapter(client, contentMutationProjectPath{})
	adapter := &flowUpdateAdapter{flows: flows, changes: resourceflow.NewMutationAdapter(client)}
	resolved, err := adapter.ResolveFlow(context.Background(), identity.Selector{LUID: "flow-1"})
	if err != nil || resolved.Name != "Daily Prep" {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	result, err := adapter.UpdateFlow(context.Background(), flowupdate.Request{LUID: "flow-1", OwnerLUID: &owner})
	if err != nil || result.FlowLUID != "flow-1" || result.FlowName != "Daily Prep" || result.ProjectLUID != "project-1" || result.OwnerLUID != owner || result.TableauRequestID != "request-flow" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestProjectMoveAdapterNormalizesReturnedHierarchyPath(t *testing.T) {
	parent := "parent-2"
	client := &contentMutationProjectClient{
		page:   tableauproject.Page{Number: 1, Size: 1, Total: 1, Items: []tableauproject.Project{{LUID: parent, Name: "Department"}}},
		result: tableauproject.MutationResult{Status: "succeeded", Project: tableauproject.Project{LUID: "project-1", Name: "Operations", ParentLUID: parent}, TableauRequestID: "request-project"},
	}
	projects := resourceproject.NewAdapter(client)
	adapter := projectMoveAdapter{projects: projects, changes: resourceproject.NewMutationAdapter(client)}
	result, err := adapter.MoveProject(context.Background(), "project-1", &parent)
	if err != nil || result.Project.Path != "Department/Operations" || result.Project.ParentLUID != parent || result.TableauRequestID != "request-project" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
