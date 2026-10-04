package app

import (
	"context"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cli/progress"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
)

type datasourcePullProvider struct{ commands *remoteContentCommands }

func (p datasourcePullProvider) ResolveDatasourceWorkspace(ctx context.Context, selector, environment, site string) (datasource.PullWorkspace, error) {
	workspace, err := (&workspaceRuntime{runtime: p.commands.runtime}).resolveForEnvironment(ctx, selector, environment)
	if err != nil {
		return datasource.PullWorkspace{}, capabilitySetupError("datasource.pull.workspace", "datasource.pull", environment, site, "Datasource workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	return datasource.PullWorkspace{Root: workspace.Root, Name: workspace.Name}, nil
}

func (p datasourcePullProvider) OpenDatasourcePull(ctx context.Context, alias, site string) (datasource.PullSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return datasource.PullSession{}, remoteSetupError("datasource.pull", alias, site, connection.environment, err)
	}
	origin, err := artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return datasource.PullSession{}, remoteSetupError("datasource.pull", connection.environment.Alias, connection.environment.SiteContentURL, connection.environment, err)
	}
	ports := resourcedatasource.PullPorts{Adapter: connection.datasources, Lineage: connection.lineage, Manager: artifact.NewDatasourceManager(p.commands.runtime.now), Progress: func(ctx context.Context, label string) { progress.SetLabel(ctx, label) }}
	return datasource.PullSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, SiteLUID: connection.siteLUID, ServerOrigin: origin, Reader: ports, Writer: ports, Previewer: ports}, nil
}
