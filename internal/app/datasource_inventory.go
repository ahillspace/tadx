package app

import (
	"context"

	datasourceinspect "github.com/ahillspace/tadx/actions/datasource/inspect"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type datasourceDiscovery struct {
	projects *resourceproject.DiscoveryPaths
}

func (c *remoteContentCommands) ListDatasources(ctx context.Context, input datasourcelist.Input) (datasourcelist.Output, error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return datasourcelist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := datasourcelist.ValidateInput(input); err != nil {
		return datasourcelist.Output{}, err
	}
	return c.listDatasources(ctx, input, &datasourceDiscovery{})
}

func (c *remoteContentCommands) listDatasources(ctx context.Context, input datasourcelist.Input, discovery *datasourceDiscovery) (result datasourcelist.Output, resultErr error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return datasourcelist.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := datasourcelist.ValidateInput(input); err != nil {
		return datasourcelist.Output{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return datasourcelist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheDatasourceListReader{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := datasourcelist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableaudatasource.ListFilter(tableaudatasource.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag, UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore})
	if err != nil {
		return datasourcelist.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourcelist.Output{}, remoteSetupError("datasource.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All && input.ProjectLUID != "" {
		if discovery.projects == nil {
			discovery.projects = resourceproject.NewDiscoveryPaths(connection.projects)
		}
		output, err := datasourcelist.New(datasourceListReader{adapter: connection.datasources, projects: discovery.projects}).Execute(ctx, input)
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
			return datasourcelist.Output{}, inventoryRefreshError("datasource.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := datasourcelist.New(reader).Execute(ctx, input)
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
	output, err := datasourcelist.New(datasourceListReader{adapter: connection.datasources, projects: discovery.projects}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func datasourceListIsUnfiltered(input datasourcelist.Input) bool {
	return input.Name == "" && input.OwnerName == "" && input.ProjectLUID == "" && input.ProjectName == "" && input.Type == "" && input.Tag == "" && input.UpdatedAfter == "" && input.UpdatedBefore == ""
}

func (c *remoteContentCommands) InspectDatasource(ctx context.Context, input datasourceinspect.Input) (datasourceinspect.Output, error) {
	if err := datasourceinspect.ValidateInput(input); err != nil {
		return datasourceinspect.Output{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return datasourceinspect.Output{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheDatasourceGetResolver{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := datasourceinspect.New(resolver).Execute(ctx, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourceinspect.Output{}, remoteSetupError("datasource.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := datasourceinspect.New(datasourceGetResolver{connection.datasources}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	upstream, upstreamErr := connection.metadataAssets.DatasourceUpstream(ctx, output.Datasource.LUID)
	if upstreamErr != nil {
		// A Metadata API permission/license failure does not erase the REST result.
		output.Datasource.Upstream = &datasourceinspect.Upstream{Status: "unavailable", Help: commandhint.Environment(input.Environment, "catalog", "audit", "--type", "datasource", "--id", output.Datasource.LUID)}
	} else {
		output.Datasource.Upstream = &datasourceinspect.Upstream{Status: "observed", Databases: upstream.Databases, Tables: upstream.Tables, Complete: upstream.Complete, ObservedAt: upstream.ObservedAt}
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

func (r datasourceListReader) ListDatasources(ctx context.Context, input datasourcelist.PageRequest) (datasourcelist.Page, error) {
	page, err := r.adapter.ListDatasources(ctx, tableaudatasource.ListRequest{
		PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName,
		ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
	})
	if err != nil {
		return datasourcelist.Page{}, err
	}
	paths := map[string]string{}
	if r.projects != nil && len(page.Items) > 0 {
		ids := make([]string, len(page.Items))
		for i, item := range page.Items {
			ids[i] = item.ProjectLUID
		}
		paths, err = r.projects.ResolveProjectPaths(ctx, ids)
		if err != nil {
			return datasourcelist.Page{}, err
		}
	}
	items := make([]datasourcelist.Datasource, len(page.Items))
	for index, item := range page.Items {
		items[index] = datasourceListItem(item)
		if path, ok := paths[item.ProjectLUID]; ok {
			items[index].ProjectPath = path
		}
	}
	return datasourcelist.Page{Number: page.Number, Size: page.Size, Total: page.Total, Datasources: items, RequestID: page.RequestID}, err
}

func datasourceListItem(item resourcedatasource.Datasource) datasourcelist.Datasource {
	return datasourcelist.Datasource{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: item.ProjectPath,
		Type: item.Type, ContentURL: item.ContentURL, Description: item.Description, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement,
	}
}

type datasourceGetResolver struct{ adapter *resourcedatasource.Adapter }

func (r datasourceGetResolver) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourceinspect.Datasource, error) {
	item, err := r.adapter.ResolveDatasource(ctx, selector)
	return datasourceinspect.Datasource{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath,
		Type: item.Type, ContentURL: item.ContentURL, Description: item.Description, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement, RequestID: item.RequestID,
	}, err
}
