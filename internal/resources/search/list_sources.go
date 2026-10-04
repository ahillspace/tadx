package search

import (
	"context"
	"fmt"
)

// ListSources binds target-scoped native list operations to bounded search.
type ListSources map[string]func(context.Context, string, int) (Page, error)

// List reads one typed native page without owning its workflow or projection.
func (s ListSources) List(ctx context.Context, kind, cursor string, limit int) (Page, error) {
	read := s[kind]
	if read == nil {
		return Page{}, fmt.Errorf("unsupported search type %q", kind)
	}
	return read(ctx, cursor, limit)
}
