package app

import (
	"context"
	"fmt"
	workbookops "github.com/ahillspace/tadx/actions/workbook"

	"github.com/ahillspace/tadx/internal/cache"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func (c *remoteContentCommands) ListWorkbooks(ctx context.Context, input workbookops.ListInput) (result workbookops.ListOutput, resultErr error) {
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return workbookops.ListOutput{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	}
	if err := workbookops.ValidateListInput(input); err != nil {
		return workbookops.ListOutput{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = validateInventoryAll(input.All, result.Source)
		}
	}()
	if input.Cache || legacyInventorySnapshot(input.Cursor) {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return workbookops.ListOutput{}, err
		}
		input.Environment, input.Site = environment, site
		reader := &cacheWorkbookListReader{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := workbookops.List(ctx, reader, input)
		if err == nil {
			output.Source = reader.source
		}
		return output, err
	}
	filter, err := tableauworkbook.ListFilter(tableauworkbook.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
	if err != nil {
		return workbookops.ListOutput{}, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return workbookops.ListOutput{}, remoteSetupError("workbook.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	if input.All && input.ProjectLUID != "" {
		snapshot, err := connection.workbooks.CollectProjectWorkbooks(ctx, tableauworkbook.ListRequest{Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
		if err != nil {
			return workbookops.ListOutput{}, inventoryRefreshError("workbook.list", input.Environment, input.Site, err)
		}
		output, err := workbookops.List(ctx, workbookListReader{snapshot: &snapshot}, input)
		if err != nil {
			return output, err
		}
		output.Source = liveSource(c.runtime.now)
		return output, nil
	}
	if input.All {
		observedAt := c.runtime.now().UTC()
		inventory, err := collectResourceInventory(ctx, connection.inventory, c.cacheStore(input.Environment), tableaucache.ScopeWorkbooks, input.Environment, input.Site, observedAt, inventoryCollectionOptions{MaxConcurrency: connection.environment.CacheMaxConcurrency, Filter: filter})
		if err != nil {
			return workbookops.ListOutput{}, inventoryRefreshError("workbook.list", input.Environment, input.Site, err)
		}
		reader := inventory.memoryReader()
		reader.allowContinuation = true
		output, err := workbookops.List(ctx, reader, input)
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
	output, err := workbookops.List(ctx, workbookListReader{adapter: connection.workbooks}, input)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(c.runtime.now)
	return output, nil
}

func (c *remoteContentCommands) InspectWorkbook(ctx context.Context, input workbookops.InspectInput) (workbookops.InspectOutput, error) {
	if err := workbookops.ValidateInspectInput(input); err != nil {
		return workbookops.InspectOutput{}, err
	}
	if input.Cache {
		environment, site, err := c.resolveCacheTarget(input.Environment)
		if err != nil {
			return workbookops.InspectOutput{}, err
		}
		input.Environment, input.Site = environment, site
		resolver := &cacheWorkbookGetResolver{store: c.cacheStore(input.Environment), environment: environment, site: site}
		output, err := workbookops.Inspect(ctx, resolver, input)
		if err == nil {
			output.Source = resolver.source
		}
		return output, err
	}
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return workbookops.InspectOutput{}, remoteSetupError("workbook.inspect", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	output, err := workbookops.Inspect(ctx, connection.workbooks, input)
	if err != nil {
		return output, err
	}
	observedAt := c.runtime.now().UTC()
	output.Source = liveSource(c.runtime.now)
	entry, encodeErr := resourceEntry(input.Environment, input.Site, "workbook", output.Workbook.LUID, output.Workbook.Name, output.Workbook.ProjectPath, output.Workbook.OwnerLUID, "detail", observedAt, output.Workbook)
	if encodeErr == nil {
		writeThrough(c.cacheStore(input.Environment), []cache.ResourceEntry{entry})
	}
	return output, nil
}

type workbookInventoryAdapter interface {
	ListWorkbooks(context.Context, tableauworkbook.ListRequest) (resourceworkbook.Page, error)
}

type workbookListReader struct {
	adapter  workbookInventoryAdapter
	snapshot *resourceworkbook.Page
}

func (r workbookListReader) ListWorkbooks(ctx context.Context, input workbookops.ListPageRequest) (workbookops.ListPage, error) {
	var page resourceworkbook.Page
	var err error
	if r.snapshot != nil {
		if input.PageNumber < 1 || input.PageSize < 1 {
			return workbookops.ListPage{}, fmt.Errorf("invalid workbook snapshot page")
		}
		start := (input.PageNumber - 1) * input.PageSize
		if start > len(r.snapshot.Items) {
			return workbookops.ListPage{}, fmt.Errorf("workbook snapshot page exceeds the collected total")
		}
		end := min(start+input.PageSize, len(r.snapshot.Items))
		page = resourceworkbook.Page{Number: input.PageNumber, Size: input.PageSize, Total: r.snapshot.Total, Items: r.snapshot.Items[start:end], RequestID: r.snapshot.RequestID}
	} else {
		page, err = r.adapter.ListWorkbooks(ctx, tableauworkbook.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
	}
	items := make([]workbookops.Record, len(page.Items))
	for index, item := range page.Items {
		items[index] = item
		items[index].Tags = append([]string(nil), item.Tags...)
	}
	return workbookops.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Workbooks: items, RequestID: page.RequestID}, err
}
