package app

import (
	"context"

	datasourceinspect "github.com/ahillspace/tadx/actions/datasource/inspect"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

func (c *remoteContentCommands) ListDatasources(ctx context.Context, input datasourcelist.Input) (datasourcelist.Output, error) {
	if input.Catalog {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return datasourcelist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &catalogDatasourceListReader{store: c.catalogStore(), environment: environment, site: site}
		output, err := datasourcelist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	if input.Cursor != "" && datasourceListIsUnfiltered(input) {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return datasourcelist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &catalogDatasourceListReader{store: c.catalogStore(), environment: environment, site: site}
		output, err := datasourcelist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourcelist.Output{}, remoteSetupError("datasource.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if datasourceListIsUnfiltered(input) {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.catalogStore(), tableaucatalog.ScopeDatasources, input.Environment, input.Site, observedAt)
		if err != nil {
			return datasourcelist.Output{}, inventoryRefreshError("datasource.list", input.Environment, input.Site, err)
		}
		if inventory.catalogErr != nil {
			reader := inventoryMemoryReader{entries: inventory.entries, requestID: finalRequestID(inventory.requestIDs)}
			output, err := datasourcelist.New(reader).Execute(ctx, input)
			if err != nil {
				return output, err
			}
			output.Source = inventory.warningSource(observedAt)
			output.Help = append(output.Help, inventory.warningHelp())
			return output, nil
		}
		reader := &catalogDatasourceListReader{store: c.catalogStore(), environment: input.Environment, site: input.Site}
		output, err := datasourcelist.New(reader).Execute(ctx, input)
		if err != nil {
			return output, err
		}
		output.Source = liveInventorySource(observedAt, inventory.published.GenerationID)
		output.RequestID = finalRequestID(inventory.requestIDs)
		output.Help = append(output.Help, inventoryRefreshHelp)
		return output, nil
	}
	output, err := datasourcelist.New(datasourceListReader{connection.datasources}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	projectIDs := make([]string, len(output.Datasources))
	for index, item := range output.Datasources {
		projectIDs[index] = item.ProjectLUID
	}
	paths, pathErr := connection.projects.ResolveProjectPaths(ctx, projectIDs)
	if pathErr != nil {
		output.Help = append(output.Help, "Live list succeeded, but canonical project paths could not be confirmed; catalog records were not updated.")
		return output, nil
	}
	entries := make([]catalog.ResourceEntry, 0, len(output.Datasources))
	for index := range output.Datasources {
		item := &output.Datasources[index]
		item.ProjectPath = paths[item.ProjectLUID]
		entry, encodeErr := resourceEntry(input.Environment, input.Site, "datasource", item.LUID, item.Name, item.ProjectPath, item.OwnerLUID, "summary", observedAt, item)
		if encodeErr == nil {
			entries = append(entries, entry)
		}
	}
	writeThrough(c.catalogStore(), entries)
	return output, nil
}

func datasourceListIsUnfiltered(input datasourcelist.Input) bool {
	return input.Name == "" && input.OwnerName == "" && input.ProjectName == "" && input.Type == "" && input.Tag == "" && input.UpdatedAfter == "" && input.UpdatedBefore == ""
}

func (c *remoteContentCommands) InspectDatasource(ctx context.Context, input datasourceinspect.Input) (datasourceinspect.Output, error) {
	if input.Catalog {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return datasourceinspect.Output{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &catalogDatasourceGetResolver{store: c.catalogStore(), environment: environment, site: site}
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
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "datasource", output.Datasource.LUID, output.Datasource.Name, output.Datasource.ProjectPath, output.Datasource.OwnerLUID, "detail", observedAt, output.Datasource)
	if encodeErr == nil {
		writeThrough(c.catalogStore(), []catalog.ResourceEntry{entry})
	}
	return output, nil
}

type datasourceListReader struct{ adapter *resourcedatasource.Adapter }

func (r datasourceListReader) ListDatasources(ctx context.Context, input datasourcelist.PageRequest) (datasourcelist.Page, error) {
	page, err := r.adapter.ListDatasources(ctx, tableaudatasource.ListRequest{
		PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName,
		ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
	})
	items := make([]datasourcelist.Datasource, len(page.Items))
	for index, item := range page.Items {
		items[index] = datasourceListItem(item)
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
