package app

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"

	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/commandhint"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type datasourceDiscovery struct {
	projects *resourceproject.DiscoveryPaths
}

func (c *remoteContentCommands) ListDatasources(ctx context.Context, input datasourceops.ListInput) (datasourceops.ListOutput, error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return datasourceops.ListOutput{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := datasourceops.ValidateListInput(input); err != nil {
		return datasourceops.ListOutput{}, err
	}
	return c.listDatasources(ctx, input, &datasourceDiscovery{})
}

func (c *remoteContentCommands) listDatasources(ctx context.Context, input datasourceops.ListInput, discovery *datasourceDiscovery) (result datasourceops.ListOutput, resultErr error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return datasourceops.ListOutput{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := datasourceops.ValidateListInput(input); err != nil {
		return datasourceops.ListOutput{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return datasourceops.ListOutput{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheDatasourceListReader{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := datasourceops.List(ctx, reader, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableaudatasource.ListFilter(tableaudatasource.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag, UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore})
	if err != nil {
		return datasourceops.ListOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourceops.ListOutput{}, remoteSetupError("datasource.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All && input.ProjectLUID != "" {
		if discovery.projects == nil {
			discovery.projects = resourceproject.NewDiscoveryPaths(connection.projects)
		}
		output, err := datasourceops.List(ctx, datasourceListReader{adapter: connection.datasources, projects: discovery.projects}, input)
		if err != nil {
			return output, err
		}
		output.Source = liveSource(c.runtime.now)
		return output, nil
	}
	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.cacheStore(input.Environment), tableaucache.ScopeDatasources, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CacheMaxConcurrency, Filter: filter})
		if err != nil {
			return datasourceops.ListOutput{}, inventoryRefreshError("datasource.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := datasourceops.List(ctx, reader, input)
		if err != nil {
			return output, err
		}
		if inventory.cacheErr != nil {
			output.Source = inventory.warningSource(observedAt)
			output.Help = append(output.Help, inventory.warningHelp())
		} else if inventory.filtered {
			output.Source = liveSource(c.runtime.now)
		} else {
			output.Source = liveInventorySource(observedAt, inventory.published.GenerationID)
		}
		output.RequestID = finalRequestID(inventory.requestIDs)
		return output, nil
	}
	if discovery.projects == nil {
		discovery.projects = resourceproject.NewDiscoveryPaths(connection.projects)
	}
	output, err := datasourceops.List(ctx, datasourceListReader{adapter: connection.datasources, projects: discovery.projects}, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func (c *remoteContentCommands) InspectDatasource(ctx context.Context, input datasourceops.InspectInput) (datasourceops.InspectOutput, error) {
	if err := datasourceops.ValidateInspectInput(input); err != nil {
		return datasourceops.InspectOutput{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return datasourceops.InspectOutput{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheDatasourceGetResolver{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := datasourceops.Inspect(ctx, resolver, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourceops.InspectOutput{}, remoteSetupError("datasource.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := datasourceops.Inspect(ctx, connection.datasources, input)
	if err != nil {
		return output, err
	}
	upstream, upstreamErr := connection.metadataAssets.DatasourceUpstream(ctx, output.Datasource.LUID)
	if upstreamErr != nil {
		// A Metadata API permission/license failure does not erase the REST result.
		output.Datasource.Upstream = &datasourceops.Upstream{Status: "unavailable", Help: commandhint.Environment(input.Environment, "catalog", "audit", "--type", "datasource", "--id", output.Datasource.LUID)}
	} else {
		output.Datasource.Upstream = &datasourceops.Upstream{Status: "observed", Databases: upstream.Databases, Tables: upstream.Tables, Complete: upstream.Complete, ObservedAt: upstream.ObservedAt}
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "datasource", output.Datasource.LUID, output.Datasource.Name, output.Datasource.ProjectPath, output.Datasource.OwnerLUID, "detail", observedAt, output.Datasource)
	if encodeErr == nil {
		writeThrough(c.cacheStore(input.Environment), []cache.ResourceEntry{entry})
	}
	return output, nil
}

type datasourceListReader struct {
	adapter  *resourcedatasource.Adapter
	projects interface {
		ResolveProjectPaths(context.Context, []string) (map[string]string, error)
	}
}

func (r datasourceListReader) ListDatasources(ctx context.Context, input datasourceops.ListPageRequest) (datasourceops.ListPage, error) {
	page, err := r.adapter.ListDatasources(ctx, tableaudatasource.ListRequest{
		PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName,
		ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
	})
	if err != nil {
		return datasourceops.ListPage{}, err
	}
	paths := map[string]string{}
	if r.projects != nil && len(page.Items) > 0 {
		ids := make([]string, len(page.Items))
		for i, item := range page.Items {
			ids[i] = item.ProjectLUID
		}
		paths, err = r.projects.ResolveProjectPaths(ctx, ids)
		if err != nil {
			return datasourceops.ListPage{}, err
		}
	}
	items := make([]datasourceops.Record, len(page.Items))
	for index, item := range page.Items {
		items[index] = item
		items[index].Tags = append([]string(nil), item.Tags...)
		if path, ok := paths[item.ProjectLUID]; ok {
			items[index].ProjectPath = path
		}
	}
	return datasourceops.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Datasources: items, RequestID: page.RequestID}, err
}
