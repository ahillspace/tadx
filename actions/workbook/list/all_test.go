package list

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type allInventoryReader struct {
	total, calls int
	duplicate    bool
}

func (r *allInventoryReader) ListWorkbooks(_ context.Context, input PageRequest) (Page, error) {
	r.calls++
	page := Page{Number: input.PageNumber, Size: input.PageSize, Total: r.total}
	for i := (input.PageNumber - 1) * input.PageSize; i < min(input.PageNumber*input.PageSize, r.total); i++ {
		id := fmt.Sprintf("item-%d", i)
		if r.duplicate {
			id = "duplicate"
		}
		page.Workbooks = append(page.Workbooks, Workbook{LUID: id})
	}
	return page, nil
}
func TestAllInventoryBoundsAndProgress(t *testing.T) {
	for _, total := range []int{0, 205, 10000, 10001} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			reader := &allInventoryReader{total: total}
			out, err := New(reader).Execute(context.Background(), Input{All: true})
			if total > 10000 {
				if err == nil || !strings.Contains(err.Error(), "10000") {
					t.Fatalf("expected bound failure: %v", err)
				}
			} else if err != nil || len(out.Workbooks) != total || out.Page.MoreAvailable || out.Page.NextCursor != "" {
				t.Fatalf("result=%+v err=%v", out.Page, err)
			}
			if reader.calls > 100 {
				t.Fatalf("unbounded requests: %d", reader.calls)
			}
		})
	}
	reader := &allInventoryReader{total: 205, duplicate: true}
	if _, err := New(reader).Execute(context.Background(), Input{All: true}); err == nil {
		t.Fatal("duplicate identities accepted")
	}
	for _, input := range []Input{{All: true, Limit: 1}, {All: true, Cursor: "legacy"}} {
		reader := &allInventoryReader{total: 205}
		if _, err := New(reader).Execute(context.Background(), input); err == nil || reader.calls != 0 {
			t.Fatalf("conflict accepted or read provider: %v", err)
		}
	}
}
