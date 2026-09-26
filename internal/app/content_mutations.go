package app

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"strings"

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
const literalSlashProjectPathUnavailable = "project name contains a literal slash; canonical hierarchy path is unavailable"

func normalizeSuccessfulProjectMutation(ctx context.Context, projects *resourceproject.Adapter, resolved map[string]resourceproject.Project, item tableauproject.Project) resourceproject.Project {
	result := resourceproject.Project{LUID: item.LUID, Name: item.Name, ParentLUID: item.ParentLUID, Description: item.Description, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}
	if item.Name == "" {
		return result
	}
	if strings.Contains(item.Name, "/") {
		result.PathUnavailableReason = literalSlashProjectPathUnavailable
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

func (c *remoteContentCommands) MoveWorkbook(ctx context.Context, input workbookops.MoveInput, preview bool) (workbookops.MoveOutput, error) {
	if err := workbookops.ValidateMoveInput(input); err != nil {
		return workbookops.MoveOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return workbookops.MoveOutput{}, remoteSetupError("workbook.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	return workbookops.Move(ctx, connection.workbooks, workbookMutationAdapter{connection.workbooks}, input, preview)
}

func (c *remoteContentCommands) UpdateWorkbook(ctx context.Context, input workbookops.UpdateInput, preview bool) (workbookops.UpdateOutput, error) {
	if err := workbookops.ValidateUpdateInput(input); err != nil {
		return workbookops.UpdateOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return workbookops.UpdateOutput{}, remoteSetupError("workbook.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	return workbookops.Update(ctx, connection.workbooks, workbookMutationAdapter{connection.workbooks}, input, preview)
}

func (c *remoteContentCommands) MoveDatasource(ctx context.Context, input datasourceops.MoveInput, preview bool) (datasourceops.MoveOutput, error) {
	if err := datasourceops.ValidateMoveInput(input); err != nil {
		return datasourceops.MoveOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourceops.MoveOutput{}, remoteSetupError("datasource.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := datasourceMutationAdapter{Adapter: connection.datasources, projects: connection.projects, changes: connection.datasourceChanges}
	return datasourceops.Move(ctx, adapter, adapter, input, preview)
}

func (c *remoteContentCommands) UpdateDatasource(ctx context.Context, input datasourceops.UpdateInput, preview bool) (datasourceops.UpdateOutput, error) {
	if err := datasourceops.ValidateUpdateInput(input); err != nil {
		return datasourceops.UpdateOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourceops.UpdateOutput{}, remoteSetupError("datasource.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := datasourceMutationAdapter{Adapter: connection.datasources, changes: connection.datasourceChanges}
	return datasourceops.Update(ctx, adapter, adapter, input, preview)
}

func (c *remoteContentCommands) UpdateFlow(ctx context.Context, input flowops.UpdateInput, preview bool) (flowops.UpdateOutput, error) {
	if err := flowops.ValidateUpdateInput(input); err != nil {
		return flowops.UpdateOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return flowops.UpdateOutput{}, remoteSetupError("flow.update", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := &flowUpdateAdapter{flows: connection.flows, changes: connection.flowChanges}
	return flowops.Update(ctx, adapter, adapter, input, preview)
}

func (c *remoteContentCommands) MoveProject(ctx context.Context, input projectmove.Input, preview bool) (projectmove.Output, error) {
	if err := projectmove.ValidateInput(input); err != nil {
		return projectmove.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return projectmove.Output{}, remoteSetupError("project.move", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := projectMoveAdapter{projects: connection.projects, changes: connection.projectChanges, resolved: make(map[string]resourceproject.Project)}
	out, err := projectmove.New(adapter, adapter).Execute(ctx, input, preview)
	if err == nil && out.Result != nil && out.Result.Project.Path == "" {
		out.Help = append(out.Help, projectMutationPathWarning)
	}
	return out, err
}

type workbookMutationAdapter struct{ workbooks *resourceworkbook.Adapter }

func (a workbookMutationAdapter) MoveWorkbook(ctx context.Context, luid, projectLUID string) (workbookops.MoveResult, error) {
	result, err := a.workbooks.UpdateWorkbook(ctx, tableauworkbook.UpdateRequest{LUID: luid, ProjectLUID: &projectLUID})
	return workbookops.MoveResult{Status: result.Status, WorkbookLUID: result.WorkbookLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

func (a workbookMutationAdapter) UpdateWorkbook(ctx context.Context, input workbookops.UpdateRequest) (workbookops.UpdateResult, error) {
	result, err := a.workbooks.UpdateWorkbook(ctx, tableauworkbook.UpdateRequest{LUID: input.LUID, Name: input.Name, OwnerLUID: input.OwnerLUID, Description: input.Description})
	return workbookops.UpdateResult{Status: result.Status, WorkbookLUID: result.WorkbookLUID, WorkbookName: result.WorkbookName, ProjectLUID: result.ProjectLUID, OwnerLUID: result.OwnerLUID, Description: result.Description, EvidenceSource: result.EvidenceSource, TableauRequestID: result.TableauRequestID}, err
}

type datasourceMutationAdapter struct {
	*resourcedatasource.Adapter
	projects *resourceproject.Adapter
	changes  *resourcedatasource.MutationAdapter
}

func (a datasourceMutationAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (datasourceops.Project, error) {
	item, err := a.projects.ResolveProject(ctx, selector)
	return datasourceops.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a datasourceMutationAdapter) MoveDatasource(ctx context.Context, luid, projectLUID string) (datasourceops.MoveResult, error) {
	result, err := a.changes.UpdateDatasource(ctx, tableaudatasource.UpdateRequest{LUID: luid, ProjectLUID: &projectLUID})
	return datasourceops.MoveResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

func (a datasourceMutationAdapter) UpdateDatasource(ctx context.Context, input datasourceops.UpdateRequest) (datasourceops.UpdateResult, error) {
	result, err := a.changes.UpdateDatasource(ctx, tableaudatasource.UpdateRequest{LUID: input.LUID, Name: input.Name, OwnerLUID: input.OwnerLUID})
	return datasourceops.UpdateResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, ProjectLUID: result.ProjectLUID, OwnerLUID: result.OwnerLUID, TableauRequestID: result.TableauRequestID}, err
}

type flowUpdateAdapter struct {
	flows    *resourceflow.Adapter
	changes  *resourceflow.MutationAdapter
	resolved *flowops.Record
}

func (a *flowUpdateAdapter) ResolveFlow(ctx context.Context, selector identity.Selector) (flowops.Record, error) {
	item, err := a.flows.ResolveFlow(ctx, selector)
	if err == nil {
		a.resolved = &item
	}
	return item, err
}

func (a *flowUpdateAdapter) UpdateFlow(ctx context.Context, input flowops.UpdateRequest) (flowops.UpdateResult, error) {
	result, err := a.changes.UpdateFlow(ctx, tableauflow.UpdateRequest{LUID: input.LUID, OwnerLUID: input.OwnerLUID})
	output := flowops.UpdateResult{Status: result.Status, FlowLUID: result.FlowLUID, OwnerLUID: result.OwnerLUID, TableauRequestID: result.TableauRequestID}
	if a.resolved != nil && a.resolved.LUID == result.FlowLUID {
		output.FlowName = a.resolved.Name
		output.ProjectLUID = a.resolved.ProjectLUID
	}
	return output, err
}

type projectMoveAdapter struct {
	projects *resourceproject.Adapter
	changes  projectMutationClient
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
	result, err := a.changes.Update(ctx, tableauproject.UpdateRequest{LUID: luid, ParentLUID: parentLUID})
	if err != nil {
		return projectmove.Result{}, err
	}
	item := normalizeSuccessfulProjectMutation(ctx, a.projects, a.resolved, result.Project)
	return projectmove.Result{Status: result.Status, Project: toProjectMove(item), TableauRequestID: result.TableauRequestID}, nil
}

func toProjectMove(item resourceproject.Project) projectmove.Project {
	return projectmove.Project{LUID: item.LUID, Name: item.Name, Path: item.Path, PathUnavailableReason: item.PathUnavailableReason, ParentLUID: item.ParentLUID, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}
}
