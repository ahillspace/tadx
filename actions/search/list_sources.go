package search

import (
	"context"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/internal/value"
)

// ListSources binds typed complete-list operations without cross-action imports.
type ListSources map[string]func(context.Context, value.SearchRequest) (value.SearchPage, error)

// SearchPage selects the requested list and preserves its continuation parameters.
func (s ListSources) SearchPage(ctx context.Context, kind, cursor string, limit int, input value.SearchRequest) (value.SearchPage, error) {
	if s == nil {
		return value.SearchPage{}, errors.New("complete live search list services are not configured")
	}
	read := s[kind]
	if read == nil {
		return value.SearchPage{}, fmt.Errorf("unsupported complete live search type %q", kind)
	}
	input.Cursor, input.Limit = cursor, limit
	return read(ctx, input)
}
