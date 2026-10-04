package definition

import (
	"context"

	"github.com/ahillspace/tadx/internal/value"
)

// ListSearch validates target-bound continuation and projects native definitions.
func ListSearch(ctx context.Context, reader ListReader, input ListInput) (value.SearchPage, error) {
	if err := ListValidateInput(&input); err != nil {
		return value.SearchPage{}, err
	}
	if err := ListValidateContinuation(input); err != nil {
		return value.SearchPage{}, err
	}
	out, err := List(ctx, reader, input)
	items := make([]value.SearchItem, len(out.Definitions))
	for i, item := range out.Definitions {
		items[i] = value.SearchItem{LUID: item.LUID, Type: "definition", Name: item.Name}
	}
	return value.SearchPage{Items: items, NextCursor: out.Page.NextCursor}, err
}
