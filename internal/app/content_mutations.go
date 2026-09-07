package app

import (
	"context"
	"strings"

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

const projectMutationPathWarning = "Project mutation succeeded, but its canonical hierarchy path could not be confirmed; inspect the project by LUID."

func normalizeSuccessfulProjectMutation(ctx context.Context, projects *resourceproject.Adapter, resolved map[string]resourceproject.Project, item tableauproject.Project) resourceproject.Project {
	result := resourceproject.Project{LUID: item.LUID, Name: item.Name, ParentLUID: item.ParentLUID, Description: item.Description, ContentPermissions: item.ContentPermissions}
	if item.Name == "" || strings.Contains(item.Name, "/") {
		return result
	}
	if item.ParentLUID == "" {
		result.Path = item.Name
		return result
	}
	if parent, ok := resolved[item.ParentLUID]; ok && parent.Path != "" {
		result.Path = parent.Path + "/" + item.Name
		return result
	}
	if source, ok := resolved[item.LUID]; ok && source.ParentLUID == item.ParentLUID && strings.HasSuffix(source.Path, "/"+source.Name) {
		result.Path = strings.TrimSuffix(source.Path, source.Name) + item.Name
		return result
	}
	if projects != nil {
		if enriched, err := projects.NormalizeMutationProject(ctx, item); err == nil {
			return enriched
		}
	}
	return result
}

func (c *remoteContentCommands) MoveWorkbook(ctx context.Context, input workbookmove.Input, preview bool) (workbookmove.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return workbookmove.Output{}, remoteSetupError("workbook.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := workbookMutationAdapter{workbooks: connection.workbooks}
	return workbookmove.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) UpdateWorkbook(ctx context.Context, input workbookupdate.Input, preview bool) (workbookupdate.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return workbookupdate.Output{}, remoteSetupError("workbook.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := workbookUpdateAdapter{workbooks: connection.workbooks}
	return workbookupdate.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) MoveDatasource(ctx context.Context, input datasourcemove.Input, preview bool) (datasourcemove.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourcemove.Output{}, remoteSetupError("datasource.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := datasourceMutationAdapter{datasources: connection.datasources, projects: connection.projects, changes: connection.datasourceChanges}
	return datasourcemove.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) UpdateDatasource(ctx context.Context, input datasourceupdate.Input, preview bool) (datasourceupdate.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourceupdate.Output{}, remoteSetupError("datasource.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := datasourceUpdateAdapter{datasources: connection.datasources, changes: connection.datasourceChanges}
	return datasourceupdate.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) UpdateFlow(ctx context.Context, input flowupdate.Input, preview bool) (flowupdate.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowupdate.Output{}, remoteSetupError("flow.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := &flowUpdateAdapter{flows: connection.flows, changes: connection.flowChanges}
	return flowupdate.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *remoteContentCommands) MoveProject(ctx context.Context, input projectmove.Input, preview bool) (projectmove.Output, error) {
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return projectmove.Output{}, remoteSetupError("project.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := projectMoveAdapter{projects: connection.projects, changes: connection.projectChanges, resolved: make(map[string]resourceproject.Project)}
	out, err := projectmove.New(adapter, adapter).Execute(ctx, input, preview)
	if err == nil && out.Result != nil && out.Result.Project.Path == "" {
		out.Help = append(out.Help, projectMutationPathWarning)
	}
	return out, err
}

type workbookMutationAdapter struct{ workbooks *resourceworkbook.Adapter }

func (a workbookMutationAdapter) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookmove.Workbook, error) {
	item, err := a.workbooks.ResolveWorkbook(ctx, selector)
	return workbookmove.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}, err
}

func (a workbookMutationAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (workbookmove.Project, error) {
	item, err := a.workbooks.ResolveProject(ctx, selector)
	return workbookmove.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a workbookMutationAdapter) FindWorkbooks(ctx context.Context, name, projectLUID string) ([]workbookmove.Workbook, error) {
	items, err := a.workbooks.FindWorkbooks(ctx, name, projectLUID)
	result := make([]workbookmove.Workbook, len(items))
	for index, item := range items {
		result[index] = workbookmove.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
	}
	return result, err
}

func (a workbookMutationAdapter) MoveWorkbook(ctx context.Context, luid, projectLUID string) (workbookmove.Result, error) {
	result, err := a.workbooks.UpdateWorkbook(ctx, tableauworkbook.UpdateRequest{LUID: luid, ProjectLUID: &projectLUID})
	return workbookmove.Result{Status: result.Status, WorkbookLUID: result.WorkbookLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

type workbookUpdateAdapter struct{ workbooks *resourceworkbook.Adapter }

func (a workbookUpdateAdapter) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookupdate.Workbook, error) {
	item, err := a.workbooks.ResolveWorkbook(ctx, selector)
	return workbookupdate.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}, err
}

func (a workbookUpdateAdapter) FindWorkbooks(ctx context.Context, name, projectLUID string) ([]workbookupdate.Workbook, error) {
	items, err := a.workbooks.FindWorkbooks(ctx, name, projectLUID)
	result := make([]workbookupdate.Workbook, len(items))
	for index, item := range items {
		result[index] = workbookupdate.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
	}
	return result, err
}

func (a workbookUpdateAdapter) UpdateWorkbook(ctx context.Context, input workbookupdate.Request) (workbookupdate.Result, error) {
	result, err := a.workbooks.UpdateWorkbook(ctx, tableauworkbook.UpdateRequest{LUID: input.LUID, Name: input.Name, OwnerLUID: input.OwnerLUID})
	return workbookupdate.Result{Status: result.Status, WorkbookLUID: result.WorkbookLUID, WorkbookName: result.WorkbookName, ProjectLUID: result.ProjectLUID, OwnerLUID: result.OwnerLUID, TableauRequestID: result.TableauRequestID}, err
}

type datasourceMutationAdapter struct {
	datasources *resourcedatasource.Adapter
	projects    *resourceproject.Adapter
	changes     *resourcedatasource.MutationAdapter
}

func (a datasourceMutationAdapter) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourcemove.Datasource, error) {
	item, err := a.datasources.ResolveDatasource(ctx, selector)
	return datasourcemove.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}, err
}

func (a datasourceMutationAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (datasourcemove.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return datasourcemove.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a datasourceMutationAdapter) FindDatasources(ctx context.Context, name, projectLUID string) ([]datasourcemove.Datasource, error) {
	items, err := a.datasources.FindDatasources(ctx, name, projectLUID)
	result := make([]datasourcemove.Datasource, len(items))
	for index, item := range items {
		result[index] = datasourcemove.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
	}
	return result, err
}

func (a datasourceMutationAdapter) MoveDatasource(ctx context.Context, luid, projectLUID string) (datasourcemove.Result, error) {
	result, err := a.changes.UpdateDatasource(ctx, tableaudatasource.UpdateRequest{LUID: luid, ProjectLUID: &projectLUID})
	return datasourcemove.Result{Status: result.Status, DatasourceLUID: result.DatasourceLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

type datasourceUpdateAdapter struct {
	datasources *resourcedatasource.Adapter
	changes     *resourcedatasource.MutationAdapter
}

func (a datasourceUpdateAdapter) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourceupdate.Datasource, error) {
	item, err := a.datasources.ResolveDatasource(ctx, selector)
	return datasourceupdate.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}, err
}

func (a datasourceUpdateAdapter) FindDatasources(ctx context.Context, name, projectLUID string) ([]datasourceupdate.Datasource, error) {
	items, err := a.datasources.FindDatasources(ctx, name, projectLUID)
	result := make([]datasourceupdate.Datasource, len(items))
	for index, item := range items {
		result[index] = datasourceupdate.Datasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
	}
	return result, err
}

func (a datasourceUpdateAdapter) UpdateDatasource(ctx context.Context, input datasourceupdate.Request) (datasourceupdate.Result, error) {
	result, err := a.changes.UpdateDatasource(ctx, tableaudatasource.UpdateRequest{LUID: input.LUID, Name: input.Name, OwnerLUID: input.OwnerLUID})
	return datasourceupdate.Result{Status: result.Status, DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, ProjectLUID: result.ProjectLUID, OwnerLUID: result.OwnerLUID, TableauRequestID: result.TableauRequestID}, err
}

type flowUpdateAdapter struct {
	flows    *resourceflow.Adapter
	changes  *resourceflow.MutationAdapter
	resolved *flowupdate.Flow
}

func (a *flowUpdateAdapter) ResolveFlow(ctx context.Context, selector identity.Selector) (flowupdate.Flow, error) {
	item, err := a.flows.ResolveFlow(ctx, selector)
	result := flowupdate.Flow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
	if err == nil {
		a.resolved = &result
	}
	return result, err
}

func (a *flowUpdateAdapter) UpdateFlow(ctx context.Context, input flowupdate.Request) (flowupdate.Result, error) {
	result, err := a.changes.UpdateFlow(ctx, tableauflow.UpdateRequest{LUID: input.LUID, OwnerLUID: input.OwnerLUID})
	output := flowupdate.Result{Status: result.Status, FlowLUID: result.FlowLUID, OwnerLUID: result.OwnerLUID, TableauRequestID: result.TableauRequestID}
	if a.resolved != nil && a.resolved.LUID == result.FlowLUID {
		output.FlowName = a.resolved.Name
		output.ProjectLUID = a.resolved.ProjectLUID
	}
	return output, err
}

type projectMoveAdapter struct {
	projects *resourceproject.Adapter
	changes  *resourceproject.MutationAdapter
	resolved map[string]resourceproject.Project
}

func (a projectMoveAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (projectmove.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	if err == nil && a.resolved != nil {
		a.resolved[item.LUID] = item
	}
	return toProjectMove(item), err
}

func (a projectMoveAdapter) FindProjectCollisions(ctx context.Context, name, parentLUID string) ([]projectmove.Project, error) {
	items, err := a.projects.FindProjectCollisions(ctx, name, parentLUID)
	result := make([]projectmove.Project, len(items))
	for index, item := range items {
		result[index] = toProjectMove(item)
	}
	return result, err
}

func (a projectMoveAdapter) MoveProject(ctx context.Context, luid string, parentLUID *string) (projectmove.Result, error) {
	result, err := a.changes.UpdateProject(ctx, tableauproject.UpdateRequest{LUID: luid, ParentLUID: parentLUID})
	if err != nil {
		return projectmove.Result{}, err
	}
	item := normalizeSuccessfulProjectMutation(ctx, a.projects, a.resolved, result.Project)
	return projectmove.Result{Status: result.Status, Project: toProjectMove(item), TableauRequestID: result.TableauRequestID}, nil
}

func toProjectMove(item resourceproject.Project) projectmove.Project {
	return projectmove.Project{LUID: item.LUID, Name: item.Name, Path: item.Path, ParentLUID: item.ParentLUID}
}
