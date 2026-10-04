package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type searchReaderFixture struct{ calls int }

func (r *searchReaderFixture) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	r.calls++
	return value.MetadataPage[value.MetadataDatabase]{Complete: true}, nil
}
func (r *searchReaderFixture) DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.calls++
	return value.MetadataPage[value.MetadataTable]{Complete: true}, nil
}
func (r *searchReaderFixture) DiscoverColumns(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.calls++
	if q.Text != "" {
		panic("fabricated column text filter")
	}
	return value.MetadataPage[value.MetadataColumn]{Items: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{LUID: "column", Name: "Sales Amount", Type: "column"}}}, Total: 1, Complete: true}, nil
}
func TestSearchColumnScopeRequired(t *testing.T) {
	r := &searchReaderFixture{}
	_, err := searchService(r).SearchCatalog(context.Background(), SearchInput{Query: "sales", Types: []string{"column"}})
	if err == nil || r.calls != 0 {
		t.Fatal("unscoped column scan accepted")
	}
}
func TestSearchColumnMatchUsesBoundedLocalScan(t *testing.T) {
	r := &searchReaderFixture{}
	out, err := searchService(r).SearchCatalog(context.Background(), SearchInput{Query: "sales", Types: []string{"column"}, TableID: "table"})
	if err != nil || len(out.Items) != 1 || !out.Complete || out.Scanned != 1 {
		t.Fatalf("%+v %v", out, err)
	}
}

type searchPartialPages struct {
	searchReaderFixture
	queries []value.MetadataQuery
	mode    string
}

func (r *searchPartialPages) DiscoverColumns(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.queries = append(r.queries, q)
	if len(r.queries) == 2 && r.mode == "error" {
		return value.MetadataPage[value.MetadataColumn]{}, errors.New("later page unavailable")
	}
	name := "match"
	if len(r.queries) == 2 && r.mode == "duplicate" {
		name = "changed"
	}
	return value.MetadataPage[value.MetadataColumn]{Items: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{LUID: "column", MetadataID: "meta", Name: name}, Table: value.MetadataIdentity{LUID: "table"}}}, NextCursor: "next", Total: 2}, nil
}
func TestSearchTraversalRetainsPartialEvidence(t *testing.T) {
	for _, mode := range []string{"error", "cursor", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			r := &searchPartialPages{mode: mode}
			out, err := searchService(r).SearchCatalog(t.Context(), SearchInput{Query: "match", Types: []string{"column"}, TableID: "table"})
			if err == nil || out.Status != "partial" || out.Complete || !out.Page.MoreAvailable || out.Page.Returned != 1 || len(out.Items) != 1 || out.Items[0].MetadataID != "meta" {
				t.Fatalf("partial=%+v error=%v", out, err)
			}
			if len(r.queries) != 2 || r.queries[1].Cursor != "next" || r.queries[1].ParentLUID != "table" {
				t.Fatalf("queries=%+v", r.queries)
			}
		})
	}
}
