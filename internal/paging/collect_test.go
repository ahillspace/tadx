package paging

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestCollectMechanicalGuards(t *testing.T) {
	for _, name := range []string{"complete", "empty-id", "duplicate", "changed-total", "stall", "repeated-token", "final-token", "bound", "cancel"} {
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
