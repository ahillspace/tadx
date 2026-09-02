package app

import (
	"context"

	workbookget "github.com/ahillspace/tadx/actions/workbook/get"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/identity"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func (c *remoteContentCommands) ListWorkbooks(ctx context.Context, input workbooklist.Input) (workbooklist.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return workbooklist.Output{}, remoteSetupError("workbook.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return workbooklist.New(workbookListReader{adapter: connection.workbooks}).Execute(ctx, input)
}

func (c *remoteContentCommands) GetWorkbook(ctx context.Context, input workbookget.Input) (workbookget.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return workbookget.Output{}, remoteSetupError("workbook.get", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return workbookget.New(workbookGetResolver{adapter: connection.workbooks}).Execute(ctx, input)
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

func (r workbookGetResolver) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookget.Workbook, error) {
	item, err := r.adapter.ResolveWorkbook(ctx, selector)
	return workbookget.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, ContentURL: item.ContentURL, UpdatedAt: item.UpdatedAt, Description: item.Description, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, Tags: append([]string(nil), item.Tags...), RequestID: item.RequestID}, err
}
