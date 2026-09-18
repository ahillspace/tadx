package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	definitiondelete "github.com/ahillspace/tadx/actions/pulse/definition/delete"
	definitioninspect "github.com/ahillspace/tadx/actions/pulse/definition/inspect"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	definitionpull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	metricdelete "github.com/ahillspace/tadx/actions/pulse/metric/delete"
	metricfollow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
	metricfollowers "github.com/ahillspace/tadx/actions/pulse/metric/followers"
	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	metricinspect "github.com/ahillspace/tadx/actions/pulse/metric/inspect"
	metriclist "github.com/ahillspace/tadx/actions/pulse/metric/list"
	metricunfollow "github.com/ahillspace/tadx/actions/pulse/metric/unfollow"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cache"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/readsource"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

const (
	pulseDefinitionKind       = "definition"
	pulseMetricKind           = "metric"
	pulseFollowerKind         = "pulse_subscription"
	pulseFollowerSnapshotKind = "pulse_follower_snapshot"
)

// pulseCommands composes Pulse actions without exposing provider contracts to Cobra.
type pulseCommands struct{ runtime *runtimeDependencies }

func newPulseCommands(runtime *runtimeDependencies) *pulseCommands {
	return &pulseCommands{runtime: runtime}
}

func (c *pulseCommands) dependencies() *pulsecli.Dependencies {
	return &pulsecli.Dependencies{
		DefinitionLister: c, DefinitionInspector: c, DefinitionPuller: c, DefinitionCreator: c, DefinitionDeleter: c, DefinitionPublisher: c,
		MetricLister: c, MetricInspector: c, MetricForker: c, MetricFollowers: c, MetricFollower: c, MetricUnfollower: c, MetricDeleter: c,
	}
}

type pulseConnection struct {
	environment config.Environment
	siteLUID    string
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
		client:      pulseClient,
		adminClient: tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL),
		schema:      resourcedatasource.NewSchemaAdapter(datasourceClient, fieldcatalog.NewClient(connection.transport, connection.session, connection.environment.URL)),
	}, nil
}

func (c *pulseCommands) cacheContent() *remoteContentCommands {
	return newRemoteContentCommands(c.runtime)
}

func (c *pulseCommands) ListPulseDefinitions(ctx context.Context, input definitionlist.Input) (definitionlist.Output, error) {
	if err := definitionlist.ValidateInput(input); err != nil {
		return definitionlist.Output{}, err
	}
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return definitionlist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
		if err := definitionlist.ValidateContinuation(input); err != nil {
			return definitionlist.Output{}, err
		}
	}
	if input.Cache {
		environment, site, err := c.cacheContent().resolveCacheTarget(input.Environment)
		if err != nil {
			return definitionlist.Output{}, capabilitySetupError("pulse.definition.list.cache.setup", "pulse.definition.list", input.Environment, "", "Cache Pulse definition setup failed.", "Verify the selected environment and cache configuration.", err)
		}
		input.Environment, input.Site = environment, site
		reader := &cachePulseDefinitionListReader{store: c.cacheContent().cacheStore(environment), environment: environment, site: site}
		output, err := definitionlist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return definitionlist.Output{}, remoteSetupError("pulse.definition.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	reader := &pulseDefinitionListAdapter{client: connection.client}
	output, err := definitionlist.New(reader).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	c.writePulseDefinitions(connection.environment, reader.items, "summary")
	return output, nil
}

func (c *pulseCommands) InspectPulseDefinition(ctx context.Context, input definitioninspect.Input) (definitioninspect.Output, error) {
	if err := definitioninspect.ValidateInput(input); err != nil {
		return definitioninspect.Output{}, err
	}
	if input.Cache {
		environment, site, err := c.cacheContent().resolveCacheTarget(input.Environment)
		if err != nil {
			return definitioninspect.Output{}, capabilitySetupError("pulse.definition.inspect.cache.setup", "pulse.definition.inspect", input.Environment, "", "Cache Pulse definition setup failed.", "Verify the selected environment and cache configuration.", err)
		}
		input.Environment, input.Site = environment, site
		reader := &cachePulseDefinitionGetReader{store: c.cacheContent().cacheStore(environment), environment: environment, site: site}
		output, err := definitioninspect.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
			output.RequestID = ""
			output.Definition.RequestID = ""
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return definitioninspect.Output{}, remoteSetupError("pulse.definition.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	reader := &pulseDefinitionGetAdapter{client: connection.client}
	output, err := definitioninspect.New(reader).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	c.writePulseDefinitions(connection.environment, []tableaupulse.Definition{reader.item}, "detail")
	return output, nil
}

func (c *pulseCommands) PullPulseDefinition(ctx context.Context, input definitionpull.Input) (definitionpull.Output, error) {
	if err := definitionpull.ValidateInput(input); err != nil {
		return definitionpull.Output{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return definitionpull.Output{}, capabilitySetupError("pulse.definition.pull.workspace", "pulse.definition.pull", input.Environment, input.Site, "Pulse definition workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = workspace.Root
	input.WorkspaceName = workspace.Name
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return definitionpull.Output{}, remoteSetupError("pulse.definition.pull", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.SiteLUID = connection.siteLUID
	input.ServerOrigin, err = artifact.NormalizeServerOrigin(connection.environment.URL)
	if err != nil {
		return definitionpull.Output{}, remoteSetupError("pulse.definition.pull", input.Environment, input.Site, connection.environment, err)
	}

	reader := &pulseDefinitionPullReader{client: connection.client}
	return definitionpull.New(reader, pulseDefinitionArtifactWriter{manager: artifact.NewPulseDefinitionManager(c.runtime.now)}).Execute(ctx, input)
}

func (c *pulseCommands) CreatePulseDefinition(ctx context.Context, input definitioncreate.Input, preview bool) (definitioncreate.Output, error) {
	if err := definitioncreate.ValidateInput(input); err != nil {
		return definitioncreate.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return definitioncreate.Output{}, remoteSetupError("pulse.definition.create", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	validator := &pulseDefinitionFieldValidator{schema: connection.schema, aggregation: input.Intent.Aggregation}
	adapter := &pulseDefinitionMutationAdapter{client: connection.client}
	return definitioncreate.New(validator, adapter, adapter).Execute(ctx, input, preview)
}

func (c *pulseCommands) DeletePulseDefinition(ctx context.Context, input definitiondelete.Input) (definitiondelete.Output, error) {
	if err := definitiondelete.ValidateInput(input); err != nil {
		return definitiondelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return definitiondelete.Output{}, remoteSetupError("pulse.definition.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := pulseDefinitionDeleteAdapter{client: connection.client}
	return definitiondelete.New(adapter, adapter).Execute(ctx, input)
}

func (c *pulseCommands) ListPulseMetrics(ctx context.Context, input metriclist.Input) (metriclist.Output, error) {
	if err := metriclist.ValidateInput(input); err != nil {
		return metriclist.Output{}, err
	}
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return metriclist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
		if err := metriclist.ValidateContinuation(input); err != nil {
			return metriclist.Output{}, err
		}
	}
	if input.Cache {
		environment, site, err := c.cacheContent().resolveCacheTarget(input.Environment)
		if err != nil {
			return metriclist.Output{}, capabilitySetupError("pulse.metric.list.cache.setup", "pulse.metric.list", input.Environment, "", "Cache Pulse metric setup failed.", "Verify the selected environment and cache configuration.", err)
		}
		input.Environment, input.Site = environment, site
		reader := &cachePulseMetricListReader{store: c.cacheContent().cacheStore(environment), environment: environment, site: site}
		output, err := metriclist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return metriclist.Output{}, remoteSetupError("pulse.metric.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	reader := &pulseMetricListAdapter{client: connection.client}
	output, err := metriclist.New(reader).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	c.writePulseMetrics(connection.environment, reader.items, "summary")
	return output, nil
}

func (c *pulseCommands) InspectPulseMetric(ctx context.Context, input metricinspect.Input) (metricinspect.Output, error) {
	if err := metricinspect.ValidateInput(input); err != nil {
		return metricinspect.Output{}, err
	}
	if input.Cache {
		environment, site, err := c.cacheContent().resolveCacheTarget(input.Environment)
		if err != nil {
			return metricinspect.Output{}, capabilitySetupError("pulse.metric.inspect.cache.setup", "pulse.metric.inspect", input.Environment, "", "Cache Pulse metric setup failed.", "Verify the selected environment and cache configuration.", err)
		}
		input.Environment, input.Site = environment, site
		reader := &cachePulseMetricGetReader{store: c.cacheContent().cacheStore(environment), environment: environment, site: site}
		output, err := metricinspect.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
			output.RequestID = ""
			output.Metric.RequestID = ""
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return metricinspect.Output{}, remoteSetupError("pulse.metric.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	reader := &pulseMetricGetAdapter{client: connection.client}
	output, err := metricinspect.New(reader).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	c.writePulseMetrics(connection.environment, []tableaupulse.Metric{reader.item}, "detail")
	return output, nil
}

func (c *pulseCommands) ForkPulseMetric(ctx context.Context, input metricfork.Input, preview bool) (metricfork.Output, error) {
	if err := metricfork.ValidateInput(input); err != nil {
		return metricfork.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return metricfork.Output{}, remoteSetupError("pulse.metric.fork", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site, input.SiteLUID = connection.environment.Alias, connection.environment.SiteContentURL, connection.siteLUID
	adapter := &pulseMetricMutationAdapter{client: connection.client, schema: connection.schema}
	return metricfork.New(adapter, adapter, adapter).Execute(ctx, input, preview)
}

func (c *pulseCommands) ListPulseMetricFollowers(ctx context.Context, input metricfollowers.Input) (metricfollowers.Output, error) {
	if err := metricfollowers.ValidateInput(input); err != nil {
		return metricfollowers.Output{}, err
	}
	if input.Cache {
		environment, site, err := c.cacheContent().resolveCacheTarget(input.Environment)
		if err != nil {
			return metricfollowers.Output{}, capabilitySetupError("pulse.metric.followers.cache.setup", "pulse.metric.followers", input.Environment, "", "Cache Pulse follower setup failed.", "Verify the selected environment and cache configuration.", err)
		}
		input.Environment, input.Site = environment, site
		reader := &cachePulseFollowerReader{store: c.cacheContent().cacheStore(environment), environment: environment, site: site}
		output, err := metricfollowers.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
			output.RequestID = ""
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return metricfollowers.Output{}, remoteSetupError("pulse.metric.followers", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	reader := &pulseFollowerAdapter{client: connection.client}
	output, err := metricfollowers.New(reader).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	if err := c.writePulseFollowers(ctx, connection.environment, input.MetricLUID, reader.items); err != nil {
		output.Warnings = append(output.Warnings, "The live follower snapshot could not be cached; the previous cached snapshot was preserved.")
	}
	return output, nil
}

func (c *pulseCommands) FollowPulseMetric(ctx context.Context, input metricfollow.Input, preview bool) (metricfollow.Output, error) {
	if err := metricfollow.ValidateInput(input); err != nil {
		return metricfollow.Output{}, err
	}
	principalCapability := "admin.user.inspect"
	if input.GroupLUID != "" {
		principalCapability = "admin.group.inspect"
	}
	if err := c.runtime.checkManagedCapability(principalCapability); err != nil {
		return metricfollow.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return metricfollow.Output{}, remoteSetupError("pulse.metric.follow", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := &pulseFollowerAdapter{client: connection.client, adminClient: connection.adminClient, checkCapability: c.runtime.checkManagedCapability}
	return metricfollow.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *pulseCommands) UnfollowPulseMetric(ctx context.Context, input metricunfollow.Input, preview bool) (metricunfollow.Output, error) {
	if err := metricunfollow.ValidateInput(input); err != nil {
		return metricunfollow.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return metricunfollow.Output{}, remoteSetupError("pulse.metric.unfollow", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	adapter := &pulseUnfollowAdapter{client: connection.client}
	return metricunfollow.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *pulseCommands) DeletePulseMetric(ctx context.Context, input metricdelete.Input) (metricdelete.Output, error) {
	if err := metricdelete.ValidateInput(input); err != nil {
		return metricdelete.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return metricdelete.Output{}, remoteSetupError("pulse.metric.delete", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := pulseMetricDeleteAdapter{client: connection.client}
	return metricdelete.New(adapter, adapter).Execute(ctx, input)
}

type pulseDefinitionListAdapter struct {
	client *tableaupulse.Client
	items  []tableaupulse.Definition
}

func (a *pulseDefinitionListAdapter) ListDefinitions(ctx context.Context, request definitionlist.PageRequest) (definitionlist.Page, error) {
	page, err := a.client.ListDefinitions(ctx, tableaupulse.PageRequest{PageSize: request.PageSize, PageToken: request.PageToken})
	if err != nil {
		return definitionlist.Page{}, err
	}
	a.items = append(a.items, page.Definitions...)
	items := make([]definitionlist.Definition, len(page.Definitions))
	for index, item := range page.Definitions {
		items[index] = definitionListItem(item)
	}
	return definitionlist.Page{Definitions: items, NextPageToken: page.NextPageToken, RequestID: page.TableauRequestID}, nil
}

type pulseDefinitionGetAdapter struct {
	client *tableaupulse.Client
	item   tableaupulse.Definition
}

func (a *pulseDefinitionGetAdapter) GetDefinition(ctx context.Context, luid string) (definitioninspect.Definition, error) {
	item, err := a.client.GetDefinition(ctx, luid)
	if err != nil {
		return definitioninspect.Definition{}, err
	}
	a.item = item
	return definitionGetItem(item)
}

type pulseDefinitionPullReader struct{ client *tableaupulse.Client }

func (r *pulseDefinitionPullReader) GetDefinition(ctx context.Context, luid string) (definitionpull.Definition, error) {
	item, err := r.client.GetDefinition(ctx, luid)
	if err != nil {
		return definitionpull.Definition{}, err
	}
	result := definitionpull.Definition{LUID: item.LUID, Name: item.Name, DatasourceLUID: item.DatasourceLUID, Configuration: append([]byte(nil), item.Configuration...), RequestID: item.TableauRequestID}
	totalBytes := len(item.Configuration)
	seenIDs, seenTokens := map[string]bool{}, map[string]bool{"": true}
	token := ""
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := r.client.ListMetrics(ctx, luid, tableaupulse.PageRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return result, err
		}
		if len(page.Metrics) > 100 {
			return result, errors.New("Pulse metric page exceeded its requested bound")
		}
		for _, summary := range page.Metrics {
			if summary.LUID == "" || seenIDs[summary.LUID] || summary.DefinitionLUID != luid {
				return result, errors.New("Pulse metric inventory contains a duplicate or mismatched identity")
			}
			seenIDs[summary.LUID] = true
			metric, err := r.client.GetMetric(ctx, summary.LUID)
			if err != nil {
				return result, err
			}
			if metric.DefinitionLUID != luid {
				return result, errors.New("Pulse metric changed definition while pulling")
			}
			specification, err := json.Marshal(metric.Specification)
			if err != nil {
				return result, err
			}
			totalBytes += len(specification) + len(metric.LUID) + len(luid) + 128
			if totalBytes > artifact.MaxPulseBundleBytes {
				return result, errors.New("Pulse bundle exceeds its 32 MiB bound; no artifact was written")
			}
			result.Metrics = append(result.Metrics, definitionpull.Metric{LUID: metric.LUID, DefinitionLUID: luid, IsDefault: summary.IsDefault || metric.IsDefault, Specification: specification})
		}
		if page.NextPageToken == "" {
			result.MetricsComplete = true
			return result, nil
		}
		if seenTokens[page.NextPageToken] || strings.TrimSpace(page.NextPageToken) == "" {
			return result, errors.New("Pulse metric inventory repeated its continuation token")
		}
		seenTokens[page.NextPageToken] = true
		token = page.NextPageToken
	}
	return result, errors.New("Pulse metric inventory exceeded 100 pages; no bundle was written")
}

type pulseDefinitionArtifactWriter struct {
	manager *artifact.PulseDefinitionManager
}

func (w pulseDefinitionArtifactWriter) WriteDefinition(ctx context.Context, input definitionpull.Artifact) (definitionpull.ArtifactResult, error) {
	bundle := artifact.PulseBundle{Version: 1, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, DefinitionLUID: input.DefinitionLUID, DatasourceReferences: []string{input.DatasourceLUID}, Definition: input.Configuration, Metrics: make([]artifact.PulseBundleMetric, 0, len(input.Metrics))}
	for _, metric := range input.Metrics {
		bundle.Metrics = append(bundle.Metrics, artifact.PulseBundleMetric{LUID: metric.LUID, DefinitionLUID: metric.DefinitionLUID, IsDefault: metric.IsDefault, Specification: metric.Specification})
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return definitionpull.ArtifactResult{}, err
	}
	result, err := w.manager.Pull(ctx, artifact.PulseDefinitionPull{
		Workspace: input.Workspace, Configuration: input.Configuration, Bundle: append(data, '\n'), Overwrite: input.Overwrite,
		Metadata: artifact.PulseDefinitionMetadata{Kind: "pulse-definition", Name: input.Name, TableauID: input.DefinitionLUID, DatasourceLUID: input.DatasourceLUID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site},
	})
	if err != nil {
		return definitionpull.ArtifactResult{}, err
	}
	return definitionpull.ArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: result.CanonicalPath, BaselineFingerprint: result.BaselineFingerprint}, nil
}

type pulseDefinitionMutationAdapter struct{ client *tableaupulse.Client }

func (a *pulseDefinitionMutationAdapter) FindDefinitions(ctx context.Context, name, datasourceLUID string) ([]definitioncreate.ExistingDefinition, error) {
	result := []definitioncreate.ExistingDefinition{}
	token := ""
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := a.client.ListDefinitions(ctx, tableaupulse.PageRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return nil, err
		}
		for _, item := range page.Definitions {
			if item.Name == name && item.DatasourceLUID == datasourceLUID {
				result = append(result, definitioncreate.ExistingDefinition{LUID: item.LUID, Name: item.Name, DatasourceLUID: item.DatasourceLUID})
			}
		}
		if page.NextPageToken == "" {
			return result, nil
		}
		token = page.NextPageToken
	}
	return nil, errors.New("Pulse definition collision scan exceeded 100 pages")
}

type pulseDefinitionDeleteAdapter struct{ client *tableaupulse.Client }

func (a pulseDefinitionDeleteAdapter) GetDefinition(ctx context.Context, luid string) (definitiondelete.Definition, error) {
	item, err := a.client.GetDefinition(ctx, luid)
	return definitiondelete.Definition{LUID: item.LUID, Name: item.Name, DatasourceLUID: item.DatasourceLUID}, err
}

func (a pulseDefinitionDeleteAdapter) DeleteDefinition(ctx context.Context, luid string) (definitiondelete.Result, error) {
	item, err := a.client.DeleteDefinition(ctx, luid)
	return definitiondelete.Result{Status: item.Status, DefinitionLUID: item.LUID, HTTPStatus: item.HTTPStatus, TableauRequestID: item.TableauRequestID}, err
}

func (a *pulseDefinitionMutationAdapter) CreateDefinition(ctx context.Context, request definitioncreate.CreateRequest) (definitioncreate.CreateResult, error) {
	var provider tableaupulse.CreateRequest
	if err := convertJSON(request, &provider); err != nil {
		return definitioncreate.CreateResult{}, fmt.Errorf("map Pulse definition create request: %w", err)
	}
	result, err := a.client.CreateDefinition(ctx, provider)
	return definitioncreate.CreateResult{Status: result.Status, DefinitionLUID: result.DefinitionLUID, DefaultMetricLUID: result.DefaultMetricLUID, DefaultMetricStatus: result.DefaultMetricStatus, TableauRequestID: result.TableauRequestID, PollRequestID: result.PollRequestID}, err
}

type pulseDefinitionFieldValidator struct {
	schema      *resourcedatasource.SchemaAdapter
	aggregation string
}

func (v *pulseDefinitionFieldValidator) ValidateDefinitionFields(ctx context.Context, references definitioncreate.FieldReferences) error {
	_, err := v.definitionFields(ctx, references, false)
	return err
}

func (v *pulseDefinitionFieldValidator) ResolveDefinitionFields(ctx context.Context, references definitioncreate.FieldReferences) (definitioncreate.FieldReferences, error) {
	return v.definitionFields(ctx, references, true)
}

func (v *pulseDefinitionFieldValidator) definitionFields(ctx context.Context, references definitioncreate.FieldReferences, resolve bool) (definitioncreate.FieldReferences, error) {
	if v == nil || v.schema == nil {
		return references, errors.New("Pulse definition field validator is not configured")
	}
	schema, err := v.schema.ReadDatasourceSchema(ctx, references.DatasourceLUID)
	if err != nil {
		return references, err
	}
	fields := make(map[string][]fieldcatalog.Field, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.ID] = append(fields[field.ID], field)
	}
	if resolve {
		selectors := make([]string, 0, 2+len(references.AllowedDimensions))
		selectors = append(selectors, references.MeasureField, references.TimeDimension)
		selectors = append(selectors, references.AllowedDimensions...)
		resolved, err := fieldcatalog.ResolveFields(schema.Fields, selectors)
		if err != nil {
			return references, err
		}
		references.MeasureField, references.TimeDimension = resolved[0].ID, resolved[1].ID
		references.AllowedDimensions = make([]string, len(resolved)-2)
		for i, field := range resolved[2:] {
			references.AllowedDimensions[i] = field.ID
		}
	}
	return references, v.validateFields(fields, references)
}

func (v *pulseDefinitionFieldValidator) validateFields(fields map[string][]fieldcatalog.Field, references definitioncreate.FieldReferences) error {
	aggregation := strings.ToUpper(strings.TrimSpace(references.Aggregation))
	if aggregation == "" {
		aggregation = strings.ToUpper(strings.TrimSpace(v.aggregation))
	}
	if aggregation == "" {
		aggregation = "AGGREGATION_SUM"
	}
	measureRoles := []string{"measure"}
	if aggregation == "AGGREGATION_COUNT" || aggregation == "AGGREGATION_COUNT_DISTINCT" {
		measureRoles = append(measureRoles, "dimension")
	}
	measure, err := exactPulseField(fields, references.MeasureField, measureRoles...)
	if err != nil {
		return fmt.Errorf("measure field: %w", err)
	}
	if _, err := exactPulseField(fields, references.TimeDimension, "date"); err != nil {
		return fmt.Errorf("time dimension: %w", err)
	}
	for _, fieldID := range references.AllowedDimensions {
		if _, err := exactPulseField(fields, fieldID, "dimension"); err != nil {
			return fmt.Errorf("allowed dimension %q: %w", fieldID, err)
		}
	}
	if measure.RequiresUserAggregation && aggregation != "AGGREGATION_USER" {
		return fmt.Errorf("field %q is already aggregated; use --aggregation USER", measure.ID)
	}
	if !measure.RequiresUserAggregation && aggregation == "AGGREGATION_USER" {
		return fmt.Errorf("field %q does not support USER aggregation", measure.ID)
	}
	if (aggregation == "AGGREGATION_SUM" || aggregation == "AGGREGATION_AVERAGE") && !numericPulseType(measure.DataType) {
		return fmt.Errorf("field %q has nonnumeric datatype %q for %s", measure.ID, measure.DataType, aggregation)
	}
	return nil
}

func exactPulseField(fields map[string][]fieldcatalog.Field, id string, roles ...string) (fieldcatalog.Field, error) {
	id = strings.TrimSpace(id)
	matches := fields[id]
	if len(matches) != 1 {
		return fieldcatalog.Field{}, fmt.Errorf("exact field ID %q matched %d fields", id, len(matches))
	}
	field := matches[0]
	if field.Excluded || field.Role == "excluded" {
		if field.ExclusionReason == "table_calc" && len(roles) > 0 && roles[0] == "measure" {
			return fieldcatalog.Field{}, fmt.Errorf("field %q is a table calculation; table calculations cannot be used as Pulse measures", id)
		}
		return fieldcatalog.Field{}, fmt.Errorf("field %q is excluded: %s", id, field.ExclusionReason)
	}
	for _, role := range roles {
		if field.Role == role {
			return field, nil
		}
	}
	return fieldcatalog.Field{}, fmt.Errorf("field %q has role %q, expected %q", id, field.Role, strings.Join(roles, " or "))
}

func numericPulseType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "NUMBER", "INTEGER", "INT", "LONG", "REAL", "FLOAT", "DOUBLE", "DECIMAL", "NUMERIC", "CURRENCY":
		return true
	default:
		return false
	}
}

type pulseMetricListAdapter struct {
	client *tableaupulse.Client
	items  []tableaupulse.Metric
}

func (a *pulseMetricListAdapter) ListMetrics(ctx context.Context, definitionLUID string, request metriclist.PageRequest) (metriclist.Page, error) {
	page, err := a.client.ListMetrics(ctx, definitionLUID, tableaupulse.PageRequest{PageSize: request.PageSize, PageToken: request.PageToken})
	if err != nil {
		return metriclist.Page{}, err
	}
	a.items = append(a.items, page.Metrics...)
	items := make([]metriclist.Metric, len(page.Metrics))
	for index, item := range page.Metrics {
		items[index] = metricListItem(item)
	}
	return metriclist.Page{Metrics: items, NextPageToken: page.NextPageToken, RequestID: page.TableauRequestID}, nil
}

type pulseMetricGetAdapter struct {
	client *tableaupulse.Client
	item   tableaupulse.Metric
}

func (a *pulseMetricGetAdapter) GetMetric(ctx context.Context, luid string) (metricinspect.Metric, error) {
	item, err := a.client.GetMetric(ctx, luid)
	if err != nil {
		return metricinspect.Metric{}, err
	}
	a.item = item
	return metricGetItem(item), nil
}

type pulseMetricMutationAdapter struct {
	client *tableaupulse.Client
	schema *resourcedatasource.SchemaAdapter
}

func (a *pulseMetricMutationAdapter) ResolveFilterFields(ctx context.Context, datasource string, selectors []string) ([]string, error) {
	if a.schema == nil {
		return nil, errors.New("Pulse filter field resolver is not configured")
	}
	schema, err := a.schema.ReadDatasourceSchema(ctx, datasource)
	if err != nil {
		return nil, err
	}
	fields, err := fieldcatalog.ResolveFields(schema.Fields, selectors)
	if err != nil {
		return nil, err
	}
	resolved := make([]string, len(fields))
	for i, field := range fields {
		if field.Excluded || field.Role != "dimension" {
			return nil, fmt.Errorf("field %q is not an eligible Pulse dimension", field.ID)
		}
		resolved[i] = field.ID
	}
	return resolved, nil
}

func (a *pulseMetricMutationAdapter) GetMetric(ctx context.Context, luid string) (metricfork.Metric, error) {
	item, err := a.client.GetMetric(ctx, luid)
	if err != nil {
		return metricfork.Metric{}, err
	}
	return metricfork.Metric{LUID: item.LUID, DefinitionLUID: item.DefinitionLUID, SiteLUID: item.SiteLUID, Specification: cloneJSONMap(item.Specification)}, nil
}

type pulseMetricDeleteAdapter struct{ client *tableaupulse.Client }

func (a pulseMetricDeleteAdapter) GetMetric(ctx context.Context, luid string) (metricdelete.Metric, error) {
	item, err := a.client.GetMetric(ctx, luid)
	var isDefault *bool
	if item.DefaultKnown {
		value := item.IsDefault
		isDefault = &value
	}
	return metricdelete.Metric{LUID: item.LUID, Name: item.Name, DefinitionLUID: item.DefinitionLUID, IsDefault: isDefault}, err
}

func (a pulseMetricDeleteAdapter) DeleteMetric(ctx context.Context, luid string) (metricdelete.Result, error) {
	item, err := a.client.DeleteMetric(ctx, luid)
	return metricdelete.Result{Status: item.Status, MetricLUID: item.LUID, HTTPStatus: item.HTTPStatus, TableauRequestID: item.TableauRequestID}, err
}

func (a *pulseMetricMutationAdapter) GetDefinition(ctx context.Context, luid string) (metricfork.Definition, error) {
	item, err := a.client.GetDefinition(ctx, luid)
	if err != nil {
		return metricfork.Definition{}, err
	}
	return metricfork.Definition{LUID: item.LUID, DatasourceLUID: item.DatasourceLUID, AllowedDimensions: append([]string(nil), item.AllowedDimensions...), AllowedGranularities: append([]string(nil), item.AllowedGranularities...), FixedFilters: append([]any(nil), item.FixedFilters...), FixedFiltersKnown: item.FixedFiltersKnown}, nil
}

func (a *pulseMetricMutationAdapter) GetOrCreateMetric(ctx context.Context, request metricfork.CreateRequest) (metricfork.CreateResult, error) {
	result, err := a.client.GetOrCreateMetric(ctx, tableaupulse.GetOrCreateRequest{DefinitionLUID: request.DefinitionLUID, Specification: cloneJSONMap(request.Specification)})
	return metricfork.CreateResult{MetricLUID: result.MetricLUID, MetricName: result.MetricName, Created: result.Created, RequestID: result.TableauRequestID}, err
}

func (a *pulseMetricMutationAdapter) ReconcileMetric(ctx context.Context, expected metricfork.ExpectedMetric) (metricfork.Reconciliation, error) {
	result, err := a.client.ReconcileMetric(ctx, tableaupulse.ExpectedMetric{MetricLUID: expected.MetricLUID, DefinitionLUID: expected.DefinitionLUID, DatasourceLUID: expected.DatasourceLUID, SiteLUID: expected.SiteLUID, Specification: cloneJSONMap(expected.Specification)})
	return metricfork.Reconciliation{Status: result.Status, Attempts: result.Attempts, OwnershipVerified: result.OwnershipVerified, RequestID: result.TableauRequestID, SpecificationVerified: result.SpecificationVerified, SavedSpecification: cloneJSONMap(result.Metric.Specification), MetricRequestID: result.Metric.TableauRequestID, DefinitionRequestID: result.Definition.TableauRequestID, SavedDefinition: metricfork.SavedDefinition{LUID: result.Definition.LUID, Name: result.Definition.Name, DatasourceLUID: result.Definition.DatasourceLUID}}, err
}

type pulseFollowerAdapter struct {
	checkCapability func(string) error
	client          *tableaupulse.Client
	adminClient     *tableauadmin.Client
	items           []tableaupulse.Subscription
}

func (a *pulseFollowerAdapter) ResolveMetric(ctx context.Context, luid string) (metricfollow.Metric, error) {
	item, err := a.client.GetMetric(ctx, luid)
	if err != nil {
		return metricfollow.Metric{}, err
	}
	return metricfollow.Metric{LUID: item.LUID}, nil
}

func (a *pulseFollowerAdapter) ResolveUser(ctx context.Context, luid string) (metricfollow.User, error) {
	if a.checkCapability != nil {
		if err := a.checkCapability("admin.user.inspect"); err != nil {
			return metricfollow.User{}, err
		}
	}
	item, err := a.adminClient.GetUser(ctx, luid)
	if err != nil {
		return metricfollow.User{}, err
	}
	return metricfollow.User{LUID: item.LUID}, nil
}

func (a *pulseFollowerAdapter) ResolveGroup(ctx context.Context, luid string) (metricfollow.Group, error) {
	if a.checkCapability != nil {
		if err := a.checkCapability("admin.group.inspect"); err != nil {
			return metricfollow.Group{}, err
		}
	}
	if _, err := a.adminClient.ListGroupUsers(ctx, luid, tableauadmin.PageRequest{PageNumber: 1, PageSize: 1}); err != nil {
		return metricfollow.Group{}, err
	}
	return metricfollow.Group{LUID: luid}, nil
}

func (a *pulseFollowerAdapter) GetMetric(ctx context.Context, luid string) (metricfollowers.Metric, error) {
	item, err := a.client.GetMetric(ctx, luid)
	if err != nil {
		return metricfollowers.Metric{}, err
	}
	return metricfollowers.Metric{LUID: item.LUID, RequestID: item.TableauRequestID}, nil
}

func (a *pulseFollowerAdapter) ListSubscriptions(ctx context.Context, metricLUID string) ([]metricfollowers.Subscription, error) {
	items, err := a.client.ListSubscriptions(ctx, metricLUID)
	if err != nil {
		return nil, err
	}
	a.items = append([]tableaupulse.Subscription(nil), items...)
	result := make([]metricfollowers.Subscription, len(items))
	for index, item := range items {
		result[index] = metricfollowers.Subscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID, FollowerName: item.FollowerName, RequestID: item.TableauRequestID}
	}
	return result, nil
}

func (a *pulseFollowerAdapter) CreateSubscription(ctx context.Context, request metricfollow.CreateRequest) (metricfollow.CreateResult, error) {
	result, err := a.client.CreateSubscription(ctx, tableaupulse.CreateSubscriptionRequest{MetricLUID: request.MetricLUID, FollowerType: request.FollowerType, FollowerLUID: request.FollowerLUID})
	return metricfollow.CreateResult{Status: result.Status, SubscriptionLUID: result.SubscriptionLUID, RequestID: result.TableauRequestID}, err
}

func (a *pulseFollowerAdapter) DeleteSubscription(ctx context.Context, luid string) error {
	return a.client.DeleteSubscription(ctx, luid)
}

type pulseUnfollowAdapter struct{ client *tableaupulse.Client }

func (a *pulseUnfollowAdapter) ListSubscriptions(ctx context.Context, metricLUID string) ([]metricunfollow.Subscription, error) {
	items, err := a.client.ListSubscriptions(ctx, metricLUID)
	if err != nil {
		return nil, err
	}
	result := make([]metricunfollow.Subscription, len(items))
	for index, item := range items {
		result[index] = metricunfollow.Subscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID, FollowerName: item.FollowerName}
	}
	return result, nil
}

func (a *pulseUnfollowAdapter) DeleteSubscription(ctx context.Context, luid string) error {
	return a.client.DeleteSubscription(ctx, luid)
}

func definitionListItem(item tableaupulse.Definition) definitionlist.Definition {
	return definitionlist.Definition{LUID: item.LUID, Name: item.Name, Description: item.Description, DatasourceLUID: item.DatasourceLUID, MeasureField: item.MeasureField, Aggregation: item.Aggregation, TimeDimension: item.TimeDimension, AllowedDimensions: append([]string(nil), item.AllowedDimensions...)}
}

func definitionGetItem(item tableaupulse.Definition) (definitioninspect.Definition, error) {
	configuration := map[string]any{}
	if len(item.Configuration) != 0 {
		if err := json.Unmarshal(item.Configuration, &configuration); err != nil {
			return definitioninspect.Definition{}, fmt.Errorf("decode Pulse definition configuration: %w", err)
		}
	}
	return definitioninspect.Definition{LUID: item.LUID, Name: item.Name, Description: item.Description, DatasourceLUID: item.DatasourceLUID, MeasureField: item.MeasureField, Aggregation: item.Aggregation, TimeDimension: item.TimeDimension, RunningTotal: item.RunningTotal, Temporality: item.Temporality, AllowedDimensions: append([]string(nil), item.AllowedDimensions...), AllowedGranularities: append([]string(nil), item.AllowedGranularities...), Configuration: configuration, RequestID: item.TableauRequestID}, nil
}

func metricListItem(item tableaupulse.Metric) metriclist.Metric {
	return metriclist.Metric{LUID: item.LUID, Name: item.Name, DefinitionLUID: item.DefinitionLUID, IsDefault: item.IsDefault, Specification: cloneJSONMap(item.Specification)}
}

func metricGetItem(item tableaupulse.Metric) metricinspect.Metric {
	return metricinspect.Metric{LUID: item.LUID, Name: item.Name, DefinitionLUID: item.DefinitionLUID, SiteLUID: item.SiteLUID, IsDefault: item.IsDefault, Specification: cloneJSONMap(item.Specification), Configuration: append([]byte(nil), item.Configuration...), RequestID: item.TableauRequestID}
}

type cachePulseDefinitionListReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cachePulseDefinitionListReader) ListDefinitions(ctx context.Context, request definitionlist.PageRequest) (definitionlist.Page, error) {
	offset, err := pulseCacheOffset(request.PageToken)
	if err != nil {
		return definitionlist.Page{}, err
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: pulseDefinitionKind, Offset: offset, Limit: request.PageSize})
	if err != nil {
		return definitionlist.Page{}, cacheReadError("pulse.definition.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]definitionlist.Definition, len(result.Entries))
	for index, entry := range result.Entries {
		var item tableaupulse.Definition
		if err := json.Unmarshal(entry.Payload, &item); err != nil {
			return definitionlist.Page{}, fmt.Errorf("decode cache Pulse definition %q: %w", entry.LUID, err)
		}
		items[index] = definitionListItem(item)
	}
	return definitionlist.Page{Definitions: items, NextPageToken: nextPulseCacheOffset(offset, len(items), result.Total)}, nil
}

type cachePulseDefinitionGetReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cachePulseDefinitionGetReader) GetDefinition(ctx context.Context, luid string) (definitioninspect.Definition, error) {
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: pulseDefinitionKind, LUID: luid, Limit: 1})
	if err != nil {
		return definitioninspect.Definition{}, cacheReadError("pulse.definition.inspect", r.environment, r.site, err)
	}
	r.source = cacheRecordSource(result, result.Entries[0])
	var item tableaupulse.Definition
	if err := json.Unmarshal(result.Entries[0].Payload, &item); err != nil {
		return definitioninspect.Definition{}, fmt.Errorf("decode cache Pulse definition %q: %w", luid, err)
	}
	item.TableauRequestID = ""
	return definitionGetItem(item)
}

type cachePulseMetricListReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cachePulseMetricListReader) ListMetrics(ctx context.Context, definitionLUID string, request metriclist.PageRequest) (metriclist.Page, error) {
	offset, err := pulseCacheOffset(request.PageToken)
	if err != nil {
		return metriclist.Page{}, err
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: pulseMetricKind, ProjectPath: definitionLUID, Offset: offset, Limit: request.PageSize})
	if err != nil {
		return metriclist.Page{}, cacheReadError("pulse.metric.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]metriclist.Metric, len(result.Entries))
	for index, entry := range result.Entries {
		var item tableaupulse.Metric
		if err := json.Unmarshal(entry.Payload, &item); err != nil {
			return metriclist.Page{}, fmt.Errorf("decode cache Pulse metric %q: %w", entry.LUID, err)
		}
		items[index] = metricListItem(item)
	}
	return metriclist.Page{Metrics: items, NextPageToken: nextPulseCacheOffset(offset, len(items), result.Total)}, nil
}

type cachePulseMetricGetReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *cachePulseMetricGetReader) GetMetric(ctx context.Context, luid string) (metricinspect.Metric, error) {
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: pulseMetricKind, LUID: luid, Limit: 1})
	if err != nil {
		return metricinspect.Metric{}, cacheReadError("pulse.metric.inspect", r.environment, r.site, err)
	}
	r.source = cacheRecordSource(result, result.Entries[0])
	var item tableaupulse.Metric
	if err := json.Unmarshal(result.Entries[0].Payload, &item); err != nil {
		return metricinspect.Metric{}, fmt.Errorf("decode cache Pulse metric %q: %w", luid, err)
	}
	item.TableauRequestID = ""
	return metricGetItem(item), nil
}

type cachePulseFollowerReader struct {
	store       *cache.Store
	environment string
	site        string
	source      *readsource.Metadata
	snapshot    *pulseFollowerSnapshot
}

func (r *cachePulseFollowerReader) GetMetric(ctx context.Context, luid string) (metricfollowers.Metric, error) {
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: pulseFollowerSnapshotKind, LUID: luid, Limit: 1})
	if err != nil {
		return metricfollowers.Metric{}, cacheReadError("pulse.metric.followers", r.environment, r.site, err)
	}
	r.source = cacheRecordSource(result, result.Entries[0])
	var snapshot pulseFollowerSnapshot
	if err := json.Unmarshal(result.Entries[0].Payload, &snapshot); err != nil {
		return metricfollowers.Metric{}, fmt.Errorf("decode cache Pulse follower snapshot: %w", err)
	}
	if snapshot.Version != 1 || snapshot.MetricLUID != luid || snapshot.Subscriptions == nil {
		return metricfollowers.Metric{}, errors.New("cache Pulse follower snapshot is incomplete or has an unsupported version; run the exact follower command without --cache to replace it")
	}
	r.snapshot = &snapshot
	return metricfollowers.Metric{LUID: snapshot.MetricLUID}, nil
}

func (r *cachePulseFollowerReader) ListSubscriptions(ctx context.Context, metricLUID string) ([]metricfollowers.Subscription, error) {
	if r.snapshot == nil || r.snapshot.MetricLUID != metricLUID {
		if _, err := r.GetMetric(ctx, metricLUID); err != nil {
			return nil, err
		}
	}
	items := make([]metricfollowers.Subscription, len(r.snapshot.Subscriptions))
	copy(items, r.snapshot.Subscriptions)
	return items, nil
}

func (c *pulseCommands) writePulseDefinitions(environment config.Environment, items []tableaupulse.Definition, coverage string) {
	observedAt := c.runtime.now().UTC()
	entries := make([]cache.ResourceEntry, 0, len(items))
	for _, item := range items {
		entry, err := resourceEntry(environment.Alias, environment.SiteContentURL, pulseDefinitionKind, item.LUID, item.Name, "", "", coverage, observedAt, item)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	writeThrough(c.runtime.cacheStore(environment), entries)
}

func (c *pulseCommands) writePulseMetrics(environment config.Environment, items []tableaupulse.Metric, coverage string) {
	observedAt := c.runtime.now().UTC()
	entries := make([]cache.ResourceEntry, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = item.LUID
		}
		entry, err := resourceEntry(environment.Alias, environment.SiteContentURL, pulseMetricKind, item.LUID, name, item.DefinitionLUID, "", coverage, observedAt, item)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	writeThrough(c.runtime.cacheStore(environment), entries)
}

type pulseFollowerSnapshot struct {
	Version       int                            `json:"version"`
	MetricLUID    string                         `json:"metric_luid"`
	Subscriptions []metricfollowers.Subscription `json:"subscriptions"`
}

func (c *pulseCommands) writePulseFollowers(ctx context.Context, environment config.Environment, metricLUID string, items []tableaupulse.Subscription) error {
	observedAt := c.runtime.now().UTC()
	snapshot := pulseFollowerSnapshot{Version: 1, MetricLUID: metricLUID, Subscriptions: make([]metricfollowers.Subscription, 0, len(items))}
	for _, item := range items {
		snapshot.Subscriptions = append(snapshot.Subscriptions, metricfollowers.Subscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID, FollowerName: item.FollowerName})
	}
	entry, err := resourceEntry(environment.Alias, environment.SiteContentURL, pulseFollowerSnapshotKind, metricLUID, metricLUID, "", "", "detail", observedAt, snapshot)
	if err != nil {
		return err
	}
	return c.runtime.cacheStore(environment).UpsertResources(ctx, []cache.ResourceEntry{entry})
}

func pulseCacheOffset(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cache continuation token")
	}
	return offset, nil
}

func nextPulseCacheOffset(offset, returned, total int) string {
	if returned == 0 || offset+returned >= total {
		return ""
	}
	return strconv.Itoa(offset + returned)
}

func convertJSON(input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, output)
}

func cloneJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var output map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&output) != nil {
		return nil
	}
	return output
}
