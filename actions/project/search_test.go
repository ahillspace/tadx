package project

import (
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestSearchPagePreservesIdentityContinuationAndEmptyRows(t *testing.T) {
	out := ListOutput{Projects: []ListProject{{LUID: "exact-id", Name: "Exact name", OwnerLUID: "owner", UpdatedAt: "updated"}}}
	out.Page.Total, out.Page.NextCursor, out.Page.MoreAvailable = 23, "opaque-cursor", true
	page := searchPage(out)
	want := value.SearchItem{LUID: "exact-id", Type: "project", Name: "Exact name", Owner: "owner", ModifiedAt: "updated"}
	if len(page.Items) != 1 || page.Items[0] != want || page.Total != 23 || page.NextCursor != "opaque-cursor" || !page.MoreAvailable {
		t.Fatalf("page=%+v", page)
	}
	if empty := searchPage(ListOutput{}); empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("empty=%+v", empty)
	}
}
