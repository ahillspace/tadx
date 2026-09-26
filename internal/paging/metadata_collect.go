package paging

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/ahillspace/tadx/internal/value"
)

// MetadataResult retains confirmed rows and request evidence when a later page fails.
type MetadataResult[T any] struct {
	Items            []T
	Total            int
	Complete         bool
	MoreAvailable    bool
	ObservedAt       string
	TableauRequestID string
}

// CollectMetadata traverses bounded metadata pages and preserves confirmed partial results.
// Callers own request selectors, user-facing bounds, and error and output projections.
func CollectMetadata[T any](ctx context.Context, limit int, read func(context.Context, int, string) (value.MetadataPage[T], error), identity func(T) string) (result MetadataResult[T], err error) {
	result.Items = []T{}
	cursor := ""
	seen := map[string]bool{}
	identities := map[string]T{}
	var coverage MetadataCoverage
	for pageNumber := 0; pageNumber < 1000; pageNumber++ {
		size := min(100, limit-len(result.Items))
		page, err := read(ctx, size, cursor)
		if err != nil {
			return result, err
		}
		result.ObservedAt = page.ObservedAt
		result.TableauRequestID = page.TableauRequestID
		if len(page.Items) > size || page.Total < 0 {
			return result, fmt.Errorf("inconsistent metadata page")
		}
		for _, item := range page.Items {
			key := identity(item)
			if strings.TrimSpace(key) == "" {
				return result, fmt.Errorf("metadata item has no authoritative identity")
			}
			if prior, ok := identities[key]; ok {
				if !reflect.DeepEqual(prior, item) {
					return result, fmt.Errorf("conflicting duplicate metadata identity")
				}
				continue
			}
			identities[key] = item
			result.Items = append(result.Items, item)
		}
		result.Total = page.Total
		if err := coverage.Page(page.Total, len(identities), page.NextCursor == ""); err != nil {
			return result, err
		}
		result.Complete = page.Complete && page.NextCursor == ""
		result.MoreAvailable = page.NextCursor != "" || !page.Complete
		if page.NextCursor == "" {
			return result, nil
		}
		if seen[page.NextCursor] {
			return result, fmt.Errorf("repeated metadata continuation")
		}
		seen[page.NextCursor] = true
		if len(result.Items) >= limit {
			return result, nil
		}
		cursor = page.NextCursor
	}
	return result, fmt.Errorf("metadata traversal exceeded page bound")
}
