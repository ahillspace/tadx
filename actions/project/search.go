package project

import (
	"context"

	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
)

// SearchPage projects the normal list workflow for a complete typed search.
func (s *Service) SearchPage(ctx context.Context, environment string, request value.SearchRequest) (value.SearchPage, error) {
	out, err := s.ListProjects(ctx, ListInput{Environment: environment, Cursor: request.Cursor, Limit: request.Limit, OwnerName: request.Owner})
	page := searchPage(out)
	page.TableauRequestID = out.RequestID
	return readsource.SearchPage(page, out.Source), err
}

// ListSearch preserves the native list contract for dedicated search scans.
func ListSearch(ctx context.Context, reader ListReader, input ListInput) (value.SearchPage, error) {

	out, err := ListFromReader(ctx, reader, input)
	page := searchPage(out)

	return page, err
}

func searchPage(out ListOutput) value.SearchPage {
	items := make([]value.SearchItem, len(out.Projects))
	for i, item := range out.Projects {
		items[i] = value.SearchItem{LUID: item.LUID, Type: "project", Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
	}
	return value.SearchPage{Items: items, Total: out.Page.Total, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable}
}
