package paging

import (
	"context"
	"strconv"
	"testing"
)

func TestWindowBoundsAndGuards(t *testing.T) {
	for _, scenario := range []string{"valid", "changing-total", "duplicate", "stall", "repeated-token", "final-token", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			result, err := Window(ctx, State{Number: 1, Size: 2501}, 1000, func(ctx context.Context, state State) (Page[string], error) {
				calls++
				page := Page[string]{Number: state.Number, Size: state.Size, Total: 4000}
				for i := 0; i < state.Size; i++ {
					page.Items = append(page.Items, strconv.Itoa((state.Number-1)*state.Size+i))
				}
				switch scenario {
				case "changing-total":
					page.Total += calls
				case "duplicate":
					page.Items[1] = page.Items[0]
				case "stall":
					page.Items = nil
				case "repeated-token":
					page.Token = "same"
				case "final-token":
					page.Total = 1000
					page.Token = "unexpected"
				case "cancel":
					cancel()
				}
				return page, nil
			}, func(s string) string { return s })
			if scenario == "valid" {
				if err != nil || len(result.Items) != 2501 || calls != 3 || result.Total != 4000 {
					t.Fatalf("len%d calls%d err%v", len(result.Items), calls, err)
				}
			} else if err == nil {
				t.Fatal("expected pagination error")
			}
		})
	}
}
