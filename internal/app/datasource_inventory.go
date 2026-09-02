package app

import (
	"context"

	datasourceget "github.com/ahillspace/tadx/actions/datasource/get"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

func (c *remoteContentCommands) ListDatasources(ctx context.Context, input datasourcelist.Input) (datasourcelist.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourcelist.Output{}, remoteSetupError("datasource.list", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return datasourcelist.New(datasourceListReader{connection.datasources}).Execute(ctx, input)
}

func (c *remoteContentCommands) GetDatasource(ctx context.Context, input datasourceget.Input) (datasourceget.Output, error) {
	connection, err := c.connect(ctx, input.Environment, false)
	if err != nil {
		return datasourceget.Output{}, remoteSetupError("datasource.get", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return datasourceget.New(datasourceGetResolver{connection.datasources}).Execute(ctx, input)
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
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName,
		Type: item.Type, ContentURL: item.ContentURL, Description: item.Description, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement,
	}
}

type datasourceGetResolver struct{ adapter *resourcedatasource.Adapter }

func (r datasourceGetResolver) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasourceget.Datasource, error) {
	item, err := r.adapter.ResolveDatasource(ctx, selector)
	return datasourceget.Datasource{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath,
		Type: item.Type, ContentURL: item.ContentURL, Description: item.Description, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement, RequestID: item.RequestID,
	}, err
}
