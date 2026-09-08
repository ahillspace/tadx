package datasource_test

import (
	adapter "github.com/ahillspace/tadx/internal/tableau/datasource"
	"testing"
)

func TestTypedListFilter(t *testing.T) {
	got, err := adapter.ListFilter(adapter.ListRequest{Name: "Sales", OwnerName: "Owner", ProjectName: "Project", Type: "hyper", Tag: "Tag", UpdatedAfter: "2026-09-01T00:00:00Z", UpdatedBefore: "2026-09-02T00:00:00Z"})
	if err != nil || got != "name:eq:Sales,ownerName:eq:Owner,projectName:eq:Project,type:eq:hyper,tags:eq:Tag,updatedAt:gte:2026-09-01T00:00:00Z,updatedAt:lte:2026-09-02T00:00:00Z" {
		t.Fatalf("filter=%q error=%v", got, err)
	}
	if got, err := adapter.ListFilter(adapter.ListRequest{}); err != nil || got != "" {
		t.Fatalf("empty filter=%q error=%v", got, err)
	}
	for _, value := range []string{"One,Two", "One&Two"} {
		if _, err := adapter.ListFilter(adapter.ListRequest{Name: value}); err == nil {
			t.Fatalf("accepted delimiter %q", value)
		}
	}
}

func TestTypedDatasourceFilterValidatesTimeBoundsAndContentURLs(t *testing.T) {
	for _, input := range []adapter.ListRequest{
		{UpdatedAfter: "invalid"}, {UpdatedBefore: "2026-09-01"},
		{UpdatedAfter: "2026-09-02T00:00:00Z", UpdatedBefore: "2026-09-01T00:00:00Z"},
		{UpdatedAfter: "0001-01-01T00:00:00Z", UpdatedBefore: "0000-01-01T00:00:00Z"},
		{ContentURLs: []string{""}}, {ContentURLs: []string{"One,Two"}}, {ContentURLs: []string{"One&Two"}},
	} {
		if _, err := adapter.ListFilter(input); err == nil {
			t.Fatalf("accepted invalid selectors: %+v", input)
		}
	}
	for _, test := range []struct {
		input adapter.ListRequest
		want  string
	}{
		{adapter.ListRequest{ContentURLs: []string{"one"}}, "contentUrl:eq:one"},
		{adapter.ListRequest{ContentURLs: []string{"one", "two"}}, "contentUrl:in:[one,two]"},
		{adapter.ListRequest{UpdatedAfter: "2026-09-01T00:00:00Z", UpdatedBefore: "2026-08-31T17:00:00-07:00"}, "updatedAt:gte:2026-09-01T00:00:00Z,updatedAt:lte:2026-08-31T17:00:00-07:00"},
	} {
		got, err := adapter.ListFilter(test.input)
		if err != nil || got != test.want {
			t.Fatalf("filter=%q error=%v", got, err)
		}
	}
}
