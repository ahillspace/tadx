package datasource

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type listAllInventoryReader struct {
	total, calls int
	duplicate    bool
}

func (r *listAllInventoryReader) ListDatasources(_ context.Context, input ListPageRequest) (ListPage, error) {
	r.calls++
	page := ListPage{Number: input.PageNumber, Size: input.PageSize, Total: r.total}
	for i := (input.PageNumber - 1) * input.PageSize; i < min(input.PageNumber*input.PageSize, r.total); i++ {
		id := fmt.Sprintf("item-%d", i)
		if r.duplicate {
			id = "duplicate"
		}
		page.Datasources = append(page.Datasources, Record{LUID: id})
	}
	return page, nil
}
func TestListAllInventoryBoundsAndProgress(t *testing.T) {
	for _, total := range []int{0, 205, 10000, 10001} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			reader := &listAllInventoryReader{total: total}
			out, err := List(context.Background(), reader, ListInput{All: true})
			if total > 10000 {
				if err == nil || !strings.Contains(err.Error(), "10000") {
					t.Fatalf("expected bound failure: %v", err)
				}
			} else if err != nil || len(out.Datasources) != total || out.Page.MoreAvailable || out.Page.NextCursor != "" {
				t.Fatalf("result=%+v err=%v", out.Page, err)
			}
			if reader.calls > 100 {
				t.Fatalf("unbounded requests: %d", reader.calls)
			}
		})
	}
	reader := &listAllInventoryReader{total: 205, duplicate: true}
	if _, err := List(context.Background(), reader, ListInput{All: true}); err == nil {
		t.Fatal("duplicate identities accepted")
	}
	for _, input := range []ListInput{{All: true, Limit: 1}, {All: true, Cursor: "legacy"}} {
		reader := &listAllInventoryReader{total: 205}
		if _, err := List(context.Background(), reader, input); err == nil || reader.calls != 0 {
			t.Fatalf("conflict accepted or read provider: %v", err)
		}
	}
}
