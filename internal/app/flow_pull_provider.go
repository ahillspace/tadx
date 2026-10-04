package app

import (
	"context"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cli/progress"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
)

type flowPullProvider struct{ commands *remoteContentCommands }

func (p flowPullProvider) ResolveFlowWorkspace(ctx context.Context, selector, environment, site string) (flow.PullWorkspace, error) {
	workspace, err := (&workspaceRuntime{runtime: p.commands.runtime}).resolveForEnvironment(ctx, selector, environment)
	if err != nil {
		return flow.PullWorkspace{}, capabilitySetupError("flow.pull.workspace", "flow.pull", environment, site, "Flow workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	return flow.PullWorkspace{Root: workspace.Root, Name: workspace.Name}, nil
}

func (p flowPullProvider) OpenFlowPull(ctx context.Context, alias, site string) (flow.PullSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return flow.PullSession{}, remoteSetupError("flow.pull", alias, site, connection.environment, err)
	}
	origin, err := artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return flow.PullSession{}, remoteSetupError("flow.pull", connection.environment.Alias, connection.environment.SiteContentURL, connection.environment, err)
	}
	ports := resourceflow.PullPorts{Adapter: connection.flows, Lineage: connection.lineage, Manager: artifact.NewFlowManager(p.commands.runtime.now), Progress: func(ctx context.Context, label string) { progress.SetLabel(ctx, label) }}
	return flow.PullSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, SiteLUID: connection.siteLUID, ServerOrigin: origin, Reader: ports, Writer: ports, Previewer: ports}, nil
}
