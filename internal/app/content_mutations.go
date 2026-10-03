package app

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	workbookops "github.com/ahillspace/tadx/actions/workbook"

	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

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
