package group

import (
	"context"

	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
)

// SearchPage projects the normal list workflow for a complete typed search.
func (s *Service) SearchPage(ctx context.Context, environment string, request value.SearchRequest) (value.SearchPage, error) {
	out, err := s.ListAdminGroups(ctx, ListInput{Environment: environment, Cursor: request.Cursor, Limit: request.Limit})
	page := searchPage(out)
	page.TableauRequestID = out.RequestID
	return readsource.SearchPage(page, out.Source), err
}

// ListSearch preserves the native list contract for dedicated search scans.
func ListSearch(ctx context.Context, reader ListReader, input ListInput) (value.SearchPage, error) {
	if err := ValidateListInput(&input); err != nil {
		return value.SearchPage{}, err
	}
	if err := ValidateListContinuation(&input); err != nil {
		return value.SearchPage{}, err
	}
	out, err := List(ctx, reader, input)
	page := searchPage(out)

	return page, err
}

func searchPage(out ListOutput) value.SearchPage {
	items := make([]value.SearchItem, len(out.Groups))
	for i, item := range out.Groups {
		items[i] = value.SearchItem{LUID: item.LUID, Type: "group", Name: item.Name}
	}
	return value.SearchPage{Items: items, Total: out.Page.Total, NextCursor: out.Page.NextCursor, MoreAvailable: out.Page.MoreAvailable}
}
