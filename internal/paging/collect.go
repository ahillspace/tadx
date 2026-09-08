// Package paging supplies mechanical guards for bounded in-process inventory reads.
package paging

import (
	"context"
	"errors"
	"fmt"
)

type State struct {
	Number, Size int
	Token        string
}
type Page[T any] struct {
	Number, Size, Total int
	Items               []T
	Token               string
}

// Collect keeps provider state typed. External cursor envelopes are not involved.
func Collect[T any](ctx context.Context, read func(context.Context, State) (Page[T], error), identity func(T) string) ([]T, error) {
	state := State{Number: 1, Size: 100}
	result := []T{}
	seen := map[string]bool{}
	tokens := map[string]bool{}
	total := -1
	for state.Number <= 100 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := read(ctx, state)
		if err != nil {
			return nil, err
		}
		if page.Number != state.Number || page.Size != state.Size || page.Total < 0 || len(page.Items) > state.Size || (state.Number-1)*state.Size+len(page.Items) > page.Total {
			return nil, errors.New("inventory reader returned inconsistent pagination")
		}
		if total < 0 {
			total = page.Total
		} else if total != page.Total {
			return nil, errors.New("inventory changed during pagination; retry")
		}
		for _, item := range page.Items {
			id := identity(item)
			if id == "" || seen[id] {
				return nil, errors.New("inventory returned missing or repeated identities")
			}
			seen[id] = true
			result = append(result, item)
		}
		if len(result) == total {
			if page.Token != "" {
				return nil, errors.New("inventory returned continuation after its declared final row")
			}
			return result, nil
		}
		if len(page.Items) != state.Size {
			return nil, errors.New("inventory pagination did not advance; completeness could not be established")
		}
		if page.Token != "" {
			if tokens[page.Token] {
				return nil, errors.New("inventory returned a repeated continuation token")
			}
			tokens[page.Token] = true
		}
		state.Number++
		state.Token = page.Token
	}
	return nil, fmt.Errorf("--all exceeds the 10000-record bound; use narrower filters")
}
