package list

import (
	"context"
	"github.com/ahillspace/tadx/internal/paging"
)

func (a *Action) readPage(ctx context.Context, input PageRequest) (Page, error) {
	if input.PageSize <= 100 {
		return a.reader.ListUsers(ctx, input)
	}
	requestID := ""
	window, err := paging.Window(ctx, paging.State{Number: input.PageNumber, Size: input.PageSize, Token: input.SnapshotCursor}, 100, func(ctx context.Context, state paging.State) (paging.Page[User], error) {
		page, err := a.reader.ListUsers(ctx, PageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, SiteRole: input.SiteRole})
		requestID = page.RequestID
		return paging.Page[User]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Users, Token: page.SnapshotCursor}, err
	}, func(item User) string { return item.LUID })
	return Page{Number: window.Number, Size: window.Size, Total: window.Total, Users: window.Items, RequestID: requestID, SuppressContinuation: true}, err
}
