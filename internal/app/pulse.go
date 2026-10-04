package app

import (
	"context"
	"fmt"
	"time"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	subscriptionlist "github.com/ahillspace/tadx/actions/pulse/subscription"
	"github.com/ahillspace/tadx/internal/artifact"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/inventory"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourcepulse "github.com/ahillspace/tadx/internal/resources/pulse"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

// pulseCommands composes Pulse actions without exposing provider contracts to Cobra.
type pulseCommands struct{ runtime *runtimeDependencies }

func newPulseCommands(runtime *runtimeDependencies) *pulseCommands {
	return &pulseCommands{runtime: runtime}
}

func (c *pulseCommands) dependencies() *pulsecli.Dependencies {
	definitionReads := pulsedefinition.New(pulsedefinition.Ports{Read: pulseDefinitionReadProvider{commands: c}, Pull: pulseDefinitionPullProvider{commands: c}, Mutation: pulseDefinitionMutationProvider{commands: c}, Publish: pulseDefinitionPublishProvider{commands: c}})
	metricReads := pulsemetric.New(pulsemetric.Ports{Read: pulseMetricReadProvider{commands: c}, Mutation: pulseMetricMutationProvider{commands: c}, Followers: pulseFollowerProvider{commands: c}})
	return &pulsecli.Dependencies{
		DefinitionLister: definitionReads, DefinitionInspector: definitionReads, DefinitionPuller: definitionReads, DefinitionCreator: definitionReads, DefinitionDeleter: definitionReads, DefinitionPublisher: definitionReads,
		MetricLister: metricReads, MetricInspector: metricReads, MetricForker: metricReads, MetricFollowers: metricReads, MetricFollower: metricReads, MetricUnfollower: metricReads, MetricDeleter: metricReads,
		SubscriptionLister: subscriptionlist.New(subscriptionProvider{commands: c}, c.runtime.now),
	}
}

type pulseConnection struct {
	environment config.Environment
	siteLUID    string
	userLUID    string
	client      *tableaupulse.Client
	adminClient *tableauadmin.Client
	schema      *resourcedatasource.SchemaAdapter
}

func (c *pulseCommands) connect(ctx context.Context, alias string, explicit bool) (pulseConnection, error) {
	connection, err := c.runtime.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return pulseConnection{environment: connection.environment}, err
	}
	pulseClient, err := tableaupulse.NewClient(connection.transport, connection.session, connection.environment.URL)
	if err != nil {
		return pulseConnection{environment: connection.environment}, fmt.Errorf("configure authenticated Pulse client: %w", err)
	}
	datasourceClient := tableaudatasource.NewClient(connection.transport, connection.session, connection.environment.URL)
	return pulseConnection{
		environment: connection.environment,
		siteLUID:    connection.session.SiteLUID(),
		userLUID:    connection.session.UserLUID(),
		client:      pulseClient,
		adminClient: tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL),
		schema:      resourcedatasource.NewSchemaAdapter(datasourceClient, fieldcatalog.NewClient(connection.transport, connection.session, connection.environment.URL)),
	}, nil
}

type pulseDefinitionPullProvider struct{ commands *pulseCommands }

func (p pulseDefinitionPullProvider) ResolveDefinitionWorkspace(ctx context.Context, selector, environment, site string) (pulsedefinition.PullWorkspace, error) {
	workspace, err := (&workspaceRuntime{runtime: p.commands.runtime}).resolveForEnvironment(ctx, selector, environment)
	if err != nil {
		return pulsedefinition.PullWorkspace{}, capabilitySetupError("pulse.definition.pull.workspace", "pulse.definition.pull", environment, site, "Pulse definition workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	return pulsedefinition.PullWorkspace{Root: workspace.Root, Name: workspace.Name}, nil
}

func (p pulseDefinitionPullProvider) OpenDefinitionPull(ctx context.Context, alias, site string) (pulsedefinition.PullSession, error) {
	connection, err := p.commands.connect(ctx, alias, false)
	if err != nil {
		return pulsedefinition.PullSession{}, remoteSetupError("pulse.definition.pull", alias, site, connection.environment, err)
	}
	origin, err := artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return pulsedefinition.PullSession{}, remoteSetupError("pulse.definition.pull", connection.environment.Alias, connection.environment.SiteContentURL, connection.environment, err)
	}
	ports := resourcepulse.DefinitionPullPort{Client: connection.client, Manager: artifact.NewPulseDefinitionManager(p.commands.runtime.now)}
	return pulsedefinition.PullSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, SiteLUID: connection.siteLUID, ServerOrigin: origin, Reader: ports, Writer: ports}, nil
}

type pulseDefinitionMutationProvider struct{ commands *pulseCommands }

func (p pulseDefinitionMutationProvider) OpenDefinitionMutation(ctx context.Context, alias, site, operation string) (pulsedefinition.MutationSession, error) {
	connection, err := p.commands.connect(ctx, alias, true)
	if err != nil {
		return pulsedefinition.MutationSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	return pulsedefinition.MutationSession{
		Environment: connection.environment.Alias,
		Site:        connection.environment.SiteContentURL,
		Fields:      &resourcepulse.DefinitionFieldPort{Schema: connection.schema},
		Definitions: &resourcepulse.DefinitionMutationPort{Client: connection.client},
	}, nil
}

type pulseMetricMutationProvider struct{ commands *pulseCommands }

func (p pulseMetricMutationProvider) OpenMetricMutation(ctx context.Context, alias, site, operation string) (pulsemetric.MutationSession, error) {
	connection, err := p.commands.connect(ctx, alias, true)
	if err != nil {
		return pulsemetric.MutationSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	inspect := &resourcepulse.MetricInspectPort{Client: connection.client}
	fork := &resourcepulse.MetricMutationPort{MetricInspectPort: inspect, Client: connection.client, Schema: connection.schema}
	return pulsemetric.MutationSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, SiteLUID: connection.siteLUID, Fork: fork, Delete: inspect}, nil
}

type pulseFollowerProvider struct{ commands *pulseCommands }

func (p pulseFollowerProvider) CacheFollowerTarget(alias string) (pulsemetric.FollowerTarget, error) {
	_, environment, err := p.commands.runtime.environment(alias, false)
	if err != nil {
		return pulsemetric.FollowerTarget{}, err
	}
	reader := &resourcepulse.CachedFollowerPort{
		Store:       p.commands.runtime.cacheStore(environment),
		Environment: environment.Alias, Site: environment.SiteContentURL,
		Support: resourcepulse.CacheSupport{ReadError: inventory.CacheReadError, RecordSource: inventory.CacheRecordSource},
	}
	return pulsemetric.FollowerTarget{Environment: environment.Alias, Site: environment.SiteContentURL, Reader: reader}, nil
}

func (p pulseFollowerProvider) CacheSetupError(alias string, err error) error {
	return capabilitySetupError("pulse.metric.followers.cache.setup", "pulse.metric.followers", alias, "", "Cache Pulse follower setup failed.", "Verify the selected environment and cache configuration.", err)
}

func (p pulseFollowerProvider) OpenFollowers(ctx context.Context, alias, site, operation string, explicit bool) (pulsemetric.FollowerSession, error) {
	connection, err := p.commands.connect(ctx, alias, explicit)
	if err != nil {
		return pulsemetric.FollowerSession{}, remoteSetupError(operation, alias, site, connection.environment, err)
	}
	port := &resourcepulse.MetricFollowerPort{
		MetricInspectPort: &resourcepulse.MetricInspectPort{Client: connection.client},
		Client:            connection.client, AdminClient: connection.adminClient,
		CheckCapability: p.commands.runtime.checkManagedCapability,
		Store:           p.commands.runtime.cacheStore(connection.environment),
		Environment:     connection.environment.Alias, Site: connection.environment.SiteContentURL,
		Now: p.commands.runtime.now,
	}
	return pulsemetric.FollowerSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Port: port}, nil
}

func (p pulseFollowerProvider) CheckPrincipalCapability(group bool) error {
	capability := "admin.user.inspect"
	if group {
		capability = "admin.group.inspect"
	}
	return p.commands.runtime.checkManagedCapability(capability)
}

func (p pulseFollowerProvider) Now() time.Time { return p.commands.runtime.now() }
