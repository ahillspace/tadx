package search

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type reader struct{ calls int }

func (r *reader) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	r.calls++
	return value.MetadataPage[value.MetadataDatabase]{Complete: true}, nil
}
func (r *reader) DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.calls++
	return value.MetadataPage[value.MetadataTable]{Complete: true}, nil
}
func (r *reader) DiscoverColumns(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.calls++
	if q.Text != "" {
		panic("fabricated column text filter")
	}
	return value.MetadataPage[value.MetadataColumn]{Items: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{LUID: "column", Name: "Sales Amount", Type: "column"}}}, Total: 1, Complete: true}, nil
}
func TestColumnScopeRequired(t *testing.T) {
	r := &reader{}
	_, err := Execute(context.Background(), r, Input{Query: "sales", Types: []string{"column"}})
	if err == nil || r.calls != 0 {
		t.Fatal("unscoped column scan accepted")
	}
}
func TestColumnMatchUsesBoundedLocalScan(t *testing.T) {
	r := &reader{}
	out, err := Execute(context.Background(), r, Input{Query: "sales", Types: []string{"column"}, TableID: "table"})
	if err != nil || len(out.Items) != 1 || !out.Complete || out.Scanned != 1 {
		t.Fatalf("%+v %v", out, err)
	}
}

type partialPages struct {
	reader
	queries []value.MetadataQuery
	mode    string
}

func (r *partialPages) DiscoverColumns(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
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
func TestTraversalRetainsPartialEvidence(t *testing.T) {
	for _, mode := range []string{"error", "cursor", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			r := &partialPages{mode: mode}
			out, err := Execute(t.Context(), r, Input{Query: "match", Types: []string{"column"}, TableID: "table"})
			if err == nil || out.Status != "partial" || out.Complete || !out.Page.MoreAvailable || out.Page.Returned != 1 || len(out.Items) != 1 || out.Items[0].MetadataID != "meta" {
				t.Fatalf("partial=%+v error=%v", out, err)
			}
			if len(r.queries) != 2 || r.queries[1].Cursor != "next" || r.queries[1].ParentLUID != "table" {
				t.Fatalf("queries=%+v", r.queries)
			}
		})
	}
}
