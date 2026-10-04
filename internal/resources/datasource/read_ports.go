package datasource

import (
	"context"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

type ProjectPaths interface {
	ResolveProjectPaths(context.Context, []string) (map[string]string, error)
}

// ListFilterPort validates and encodes a native datasource selection before authentication.
type ListFilterPort struct{}

func (ListFilterPort) ListFilter(input datasource.ListInput) (string, error) {
	return tableaudatasource.ListFilter(tableaudatasource.ListRequest{
		Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID,
		ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
	})
}

type ReadPorts struct {
	Adapter  *Adapter
	Projects ProjectPaths
}

func (p ReadPorts) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasource.Record, error) {
	return p.Adapter.ResolveDatasource(ctx, selector)
}

func (p ReadPorts) ListDatasources(ctx context.Context, input datasource.ListPageRequest) (datasource.ListPage, error) {
	page, err := p.Adapter.ListDatasources(ctx, tableaudatasource.ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag, UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore})
	if err != nil {
		return datasource.ListPage{}, err
	}
	paths := map[string]string{}
	if p.Projects != nil && len(page.Items) > 0 {
		ids := make([]string, len(page.Items))
		for i, item := range page.Items {
			ids[i] = item.ProjectLUID
		}
		paths, err = p.Projects.ResolveProjectPaths(ctx, ids)
		if err != nil {
			return datasource.ListPage{}, err
		}
	}
	items := make([]datasource.Record, len(page.Items))
	for index, item := range page.Items {
		items[index] = item
		items[index].Tags = append([]string(nil), item.Tags...)
		if path, ok := paths[item.ProjectLUID]; ok {
			items[index].ProjectPath = path
		}
	}
	return datasource.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Datasources: items, RequestID: page.RequestID}, nil
}

type UpstreamClient interface {
	DatasourceUpstream(context.Context, string) (metadataassets.DatasourceUpstream, error)
}
type UpstreamPort struct{ Client UpstreamClient }

func (p UpstreamPort) ReadDatasourceUpstream(ctx context.Context, luid string) (datasource.UpstreamObservation, error) {
	result, err := p.Client.DatasourceUpstream(ctx, luid)
	return datasource.UpstreamObservation{Databases: result.Databases, Tables: result.Tables, Complete: result.Complete, ObservedAt: result.ObservedAt}, err
}
