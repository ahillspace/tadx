package readsource

import (
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestSearchPagePreservesDataAndReportsObservedSource(t *testing.T) {
	for _, test := range []struct{ mode, want string }{{Tableau, "live"}, {Cache, "cache"}, {"unknown", ""}} {
		page := SearchPage(value.SearchPage{Items: []value.SearchItem{}, Total: 7, NextCursor: "opaque", TableauRequestID: "request"}, &Metadata{Mode: test.mode, CacheWarning: "publication failed"})
		if page.Source != test.want || page.Total != 7 || page.NextCursor != "opaque" || page.TableauRequestID != "request" || page.Items == nil || len(page.Warnings) != 1 || page.Warnings[0] != "publication failed" {
			t.Fatalf("page=%+v", page)
		}
	}
	if page := SearchPage(value.SearchPage{Source: "kept"}, nil); page.Source != "kept" || page.Warnings != nil {
		t.Fatalf("page=%+v", page)
	}
}
