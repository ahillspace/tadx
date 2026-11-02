package paging

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestCollectMechanicalGuards(t *testing.T) {
	for _, name := range []string{"complete", "empty-id", "duplicate", "changed-total", "wrong-number", "wrong-size", "negative-total", "stall", "repeated-token", "final-token", "bound", "cancel"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			total := 205
			if name == "final-token" {
				total = 3
			}
			if name == "bound" {
				total = 10001
			}
			calls := 0
			items, err := Collect(ctx, func(_ context.Context, s State) (Page[string], error) {
				calls++
				p := Page[string]{Number: s.Number, Size: s.Size, Total: total}
				for i := (s.Number - 1) * s.Size; i < min(s.Number*s.Size, total); i++ {
					p.Items = append(p.Items, fmt.Sprint(i))
				}
				switch name {
				case "empty-id":
					p.Items[0] = ""
				case "duplicate":
					p.Items[1] = p.Items[0]
				case "changed-total":
					if s.Number == 2 {
						p.Total++
					}
				case "wrong-number":
					p.Number++
				case "wrong-size":
					p.Size--
				case "negative-total":
					p.Total = -1
				case "stall":
					if s.Number == 2 {
						p.Items = nil
					}
				case "repeated-token":
					p.Token = "same"
				case "final-token":
					p.Token = "unexpected"
				case "cancel":
					cancel()
				}
				return p, nil
			}, func(id string) string { return id })
			if name == "complete" {
				if err != nil || len(items) != total {
					t.Fatalf("items=%d err=%v", len(items), err)
				}
			} else if err == nil {
				t.Fatal("invalid traversal succeeded")
			}
			if name == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation=%v", err)
			}
			if calls > 100 {
				t.Fatalf("unbounded provider calls=%d", calls)
			}
		})
	}
}

func TestCollectBoundedRetainsCallerScanLimits(t *testing.T) {
	for _, maxPages := range []int{2, 3} {
		calls := 0
		items, err := CollectBounded(t.Context(), 2, maxPages, func(_ context.Context, state State) (Page[string], error) {
			calls++
			page := Page[string]{Number: state.Number, Size: state.Size, Total: 5}
			for i := (state.Number - 1) * state.Size; i < min(state.Number*state.Size, 5); i++ {
				page.Items = append(page.Items, fmt.Sprint(i))
			}
			return page, nil
		}, func(item string) string { return item })
		if maxPages == 2 {
			if err == nil || items != nil || calls != 2 {
				t.Fatalf("bound ignored: items=%v err=%v calls=%d", items, err, calls)
			}
		} else if err != nil || len(items) != 5 || calls != 3 {
			t.Fatalf("valid bounded scan failed: items=%v err=%v calls=%d", items, err, calls)
		}
	}
}
