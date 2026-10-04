package app

import (
	"context"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	resourcepulse "github.com/ahillspace/tadx/internal/resources/pulse"
)

type pulseDefinitionPublishProvider struct{ commands *pulseCommands }

func (p pulseDefinitionPublishProvider) ResolveBundleWorkspace(ctx context.Context, selector, environment string) (pulsedefinition.PullWorkspace, error) {
	workspace, err := (&workspaceRuntime{runtime: p.commands.runtime}).resolveForEnvironment(ctx, selector, environment)
	if err != nil {
		return pulsedefinition.PullWorkspace{}, capabilitySetupError("pulse.definition.publish.workspace", "pulse.definition.publish", environment, "", "Pulse bundle workspace resolution failed.", "Select an exact logical workspace.", err)
	}
	return pulsedefinition.PullWorkspace{Root: workspace.Root, Name: workspace.Name}, nil
}

func (pulseDefinitionPublishProvider) BundleReader() pulsedefinition.PublishBundleReader {
	return resourcepulse.DefinitionBundleArtifactPort{}
}

func (p pulseDefinitionPublishProvider) OpenDefinitionPublish(ctx context.Context, alias, site string) (pulsedefinition.PublishSession, error) {
	connection, err := p.commands.connect(ctx, alias, true)
	if err != nil {
		return pulsedefinition.PublishSession{}, remoteSetupError("pulse.definition.publish", alias, site, connection.environment, err)
	}
	port := resourcepulse.DefinitionBundlePort{Client: connection.client, Schema: connection.schema}
	return pulsedefinition.PublishSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, SiteLUID: connection.siteLUID, Validator: port, Writer: port}, nil
}
