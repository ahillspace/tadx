package list

import (
	"context"
	"github.com/ahillspace/tadx/internal/paging"
)

func (a *Action) readPage(ctx context.Context, input PageRequest) (Page, error) {
	if input.PageSize <= 100 {
		return a.reader.ListGroups(ctx, input)
	}
	requestID := ""
	window, err := paging.Window(ctx, paging.State{Number: input.PageNumber, Size: input.PageSize, Token: input.SnapshotCursor}, 100, func(ctx context.Context, state paging.State) (paging.Page[Group], error) {
		page, err := a.reader.ListGroups(ctx, PageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, Domain: input.Domain})
		requestID = page.RequestID
		return paging.Page[Group]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Groups, Token: page.SnapshotCursor}, err
	}, func(item Group) string { return item.LUID })
	return Page{Number: window.Number, Size: window.Size, Total: window.Total, Groups: window.Items, RequestID: requestID, SuppressContinuation: true}, err
}
