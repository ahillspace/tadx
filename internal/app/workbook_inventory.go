package app

import (
	"context"

	workbookinspect "github.com/ahillspace/tadx/actions/workbook/inspect"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/identity"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func (c *remoteContentCommands) ListWorkbooks(ctx context.Context, input workbooklist.Input) (result workbooklist.Output, resultErr error) {
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Catalog {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return workbooklist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &catalogWorkbookListReader{store: c.catalogStore(), environment: environment, site: site}
		output, err := workbooklist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	if input.Cursor != "" && workbookListIsUnfiltered(input) {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return workbooklist.Output{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &catalogWorkbookListReader{store: c.catalogStore(), environment: environment, site: site}
		output, err := workbooklist.New(reader).Execute(ctx, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return workbooklist.Output{}, remoteSetupError("workbook.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if workbookListIsUnfiltered(input) {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.catalogStore(), tableaucatalog.ScopeWorkbooks, input.Environment, input.Site, observedAt)
		if err != nil {
			return workbooklist.Output{}, inventoryRefreshError("workbook.list", input.Environment, input.Site, err)
		}
		if inventory.catalogErr != nil {
			reader := inventory.memoryReader()
			reader.allowContinuation = input.All
			output, err := workbooklist.New(reader).Execute(ctx, input)
			if err != nil {
				return output, err
			}
			output.Source = inventory.warningSource(observedAt)
			output.Help = append(output.Help, inventory.warningHelp())
			return output, nil
		}
		reader := &catalogWorkbookListReader{store: c.catalogStore(), environment: input.Environment, site: input.Site}
		output, err := workbooklist.New(reader).Execute(ctx, input)
		if err != nil {
			return output, err
		}
		output.Source = liveInventorySource(observedAt, inventory.published.GenerationID)
		output.RequestID = finalRequestID(inventory.requestIDs)
		output.Help = append(output.Help, inventoryRefreshHelp)
		return output, nil
	}
	output, err := workbooklist.New(workbookListReader{adapter: connection.workbooks}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entries := make([]catalog.ResourceEntry, 0, len(output.Workbooks))
	for _, item := range output.Workbooks {
		entry, encodeErr := resourceEntry(input.Environment, input.Site, "workbook", item.LUID, item.Name, item.ProjectPath, item.OwnerLUID, "summary", observedAt, item)
		if encodeErr == nil {
			entries = append(entries, entry)
		}
	}
	writeThrough(c.catalogStore(), entries)
	return output, nil
}

func workbookListIsUnfiltered(input workbooklist.Input) bool {
	return input.Name == "" && input.OwnerName == "" && input.ProjectName == "" && input.Tag == ""
}

func (c *remoteContentCommands) InspectWorkbook(ctx context.Context, input workbookinspect.Input) (workbookinspect.Output, error) {
	if input.Catalog {
		environment, site, err := c.resolveCatalogTarget(input.Environment)
		if err != nil {
			return workbookinspect.Output{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &catalogWorkbookGetResolver{store: c.catalogStore(), environment: environment, site: site}
		output, err := workbookinspect.New(resolver).Execute(ctx, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return workbookinspect.Output{}, remoteSetupError("workbook.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := workbookinspect.New(workbookGetResolver{adapter: connection.workbooks}).Execute(ctx, input)
	if err != nil {
		return output, err
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "workbook", output.Workbook.LUID, output.Workbook.Name, output.Workbook.ProjectPath, output.Workbook.OwnerLUID, "detail", observedAt, output.Workbook)
	if encodeErr == nil {
		writeThrough(c.catalogStore(), []catalog.ResourceEntry{entry})
	}
	return output, nil
}

type workbookInventoryAdapter interface {
	ListWorkbooks(context.Context, tableauworkbook.ListRequest) (resourceworkbook.Page, error)
	ResolveWorkbook(context.Context, identity.Selector) (resourceworkbook.Workbook, error)
}

type workbookListReader struct{ adapter workbookInventoryAdapter }

func (r workbookListReader) ListWorkbooks(ctx context.Context, input workbooklist.PageRequest) (workbooklist.Page, error) {
	page, err := r.adapter.ListWorkbooks(ctx, tableauworkbook.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectName: input.ProjectName, Tag: input.Tag})
	items := make([]workbooklist.Workbook, len(page.Items))
	for index, item := range page.Items {
		items[index] = workbooklist.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...)}
	}
	return workbooklist.Page{Number: page.Number, Size: page.Size, Total: page.Total, Workbooks: items, RequestID: page.RequestID}, err
}

type workbookGetResolver struct{ adapter workbookInventoryAdapter }

func (r workbookGetResolver) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookinspect.Workbook, error) {
	item, err := r.adapter.ResolveWorkbook(ctx, selector)
	return workbookinspect.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...), RequestID: item.RequestID}, err
}
