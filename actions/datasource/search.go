package datasource

import (
	"context"

	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
)

// SearchPage projects the normal list workflow for a complete typed search.
func (s *Service) SearchPage(ctx context.Context, environment string, request value.SearchRequest) (value.SearchPage, error) {
	out, err := s.ListDatasources(ctx, ListInput{Environment: environment, Cursor: request.Cursor, Limit: request.Limit, ProjectName: request.ProjectPath, OwnerName: request.Owner})
	page := searchPage(out)
	page.TableauRequestID = out.RequestID
	return readsource.SearchPage(page, out.Source), err
}

// ListSearch preserves the native list contract for dedicated search scans.
func ListSearch(ctx context.Context, reader ListReader, input ListInput) (value.SearchPage, error) {

	out, err := List(ctx, reader, input)
	page := searchPage(out)
	for i := range page.Items {
		page.Items[i].ProjectPath = ""
	}
	return page, err
}

func searchPage(out ListOutput) value.SearchPage {
	items := make([]value.SearchItem, len(out.Datasources))
	for i, item := range out.Datasources {
		items[i] = value.SearchItem{LUID: item.LUID, Type: "datasource", Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt, ProjectPath: item.ProjectPath}
	}
	return value.SearchPage{Items: items, Total: out.Page.Total, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable}
}
