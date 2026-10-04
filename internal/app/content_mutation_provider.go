package app

import (
	"context"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
)

// contentMutationProvider constructs target-bound ports only after action validation.
type contentMutationProvider struct{ commands *remoteContentCommands }

func (p contentMutationProvider) OpenWorkbookMutation(ctx context.Context, alias, site, operation string) (workbookops.MutationSession, error) {
	connection, err := p.commands.connect(ctx, alias, true)
	if err != nil {
		return workbookops.MutationSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	port := resourceworkbook.NewMutationPort(connection.workbooks, connection.workbookChanges)
	return workbookops.MutationSession{
		Environment:  connection.environment.Alias,
		Site:         connection.environment.SiteContentURL,
		MoveResolver: port, Mover: port, UpdateResolver: port, Updater: port, DeleteResolver: port, Deleter: port,
	}, nil
}

func (p contentMutationProvider) OpenDatasourceMutation(ctx context.Context, alias, site, operation string) (datasourceops.MutationSession, error) {
	connection, err := p.commands.connect(ctx, alias, true)
	if err != nil {
		return datasourceops.MutationSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	port := resourcedatasource.NewMutationPort(connection.datasources, connection.projects, connection.datasourceNativeChanges)
	return datasourceops.MutationSession{
		Environment:  connection.environment.Alias,
		Site:         connection.environment.SiteContentURL,
		MoveResolver: port, Mover: port, UpdateResolver: port, Updater: port, DeleteResolver: port, Deleter: port,
	}, nil
}

func (p contentMutationProvider) OpenFlowMutation(ctx context.Context, alias, site, operation string) (flowops.MutationSession, error) {
	connection, err := p.commands.connect(ctx, alias, true)
	if err != nil {
		return flowops.MutationSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	port := resourceflow.NewMutationPort(connection.flows, connection.projects, connection.flowNativeChanges)
	return flowops.MutationSession{
		Environment:  connection.environment.Alias,
		Site:         connection.environment.SiteContentURL,
		MoveResolver: port, Mover: port, UpdateResolver: port, Updater: port, DeleteResolver: port, Deleter: port,
	}, nil
}
