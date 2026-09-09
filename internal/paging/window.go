package paging

import (
	"context"
	"errors"
)

// Window returns one logical bounded page using provider pages no larger than
// 1000. Large windows are cursor-free; legacy smaller pages retain their path.
func Window[T any](ctx context.Context, logical State, providerSize int, read func(context.Context, State) (Page[T], error), identity func(T) string) (Page[T], error) {
	if logical.Size < 1 || logical.Size > 10000 || logical.Number != 1 || logical.Token != "" || providerSize < 1 || providerSize > 1000 {
		return Page[T]{}, errors.New("invalid bounded inventory window")
	}
	size := min(logical.Size, providerSize)
	state := State{Number: (logical.Number-1)*(logical.Size/size) + 1, Size: size, Token: logical.Token}
	result := Page[T]{Number: logical.Number, Size: logical.Size, Total: -1}
	seen, tokens := map[string]bool{}, map[string]bool{}
	for len(result.Items) < logical.Size {
		if err := ctx.Err(); err != nil {
			return Page[T]{}, err
		}
		page, err := read(ctx, state)
		if err != nil {
			return Page[T]{}, err
		}
		if err := ctx.Err(); err != nil {
			return Page[T]{}, err
		}
		offset := (state.Number - 1) * size
		if page.Number != state.Number || page.Size != size || page.Total < 0 || len(page.Items) > size || (len(page.Items) > 0 && offset+len(page.Items) > page.Total) {
			return Page[T]{}, errors.New("inventory reader returned inconsistent pagination")
		}
		if result.Total < 0 {
			result.Total = page.Total
		} else if result.Total != page.Total {
			return Page[T]{}, errors.New("inventory changed during pagination; retry")
		}
		for _, item := range page.Items {
			id := identity(item)
			if id == "" || seen[id] {
				return Page[T]{}, errors.New("inventory returned missing or repeated identities")
			}
			seen[id] = true
			if len(result.Items) < logical.Size {
				result.Items = append(result.Items, item)
			}
		}
		result.Token = page.Token
		if offset+len(page.Items) >= page.Total {
			if page.Token != "" {
				return Page[T]{}, errors.New("inventory returned continuation after its final row")
			}
			return result, nil
		}
		if len(page.Items) != size {
			return Page[T]{}, errors.New("inventory pagination did not advance")
		}
		if page.Token != "" {
			if tokens[page.Token] {
				return Page[T]{}, errors.New("inventory returned a repeated continuation token")
			}
			tokens[page.Token] = true
		}
		state.Number++
		state.Token = page.Token
	}
	return result, nil
}
