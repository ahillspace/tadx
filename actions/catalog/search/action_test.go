package search

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
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
	return value.MetadataPage[value.MetadataColumn]{Items: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{LUID: "column", Name: "Sales Amount", Type: "column"}}}, Complete: true}, nil
}
func TestColumnScopeRequired(t *testing.T) {
	r := &reader{}
	_, err := New(r).Execute(context.Background(), Input{Query: "sales", Types: []string{"column"}})
	if err == nil || r.calls != 0 {
		t.Fatal("unscoped column scan accepted")
	}
}
func TestColumnMatchUsesBoundedLocalScan(t *testing.T) {
	r := &reader{}
	out, err := New(r).Execute(context.Background(), Input{Query: "sales", Types: []string{"column"}, TableID: "table"})
	if err != nil || len(out.Items) != 1 || !out.Complete || out.Scanned != 1 {
		t.Fatalf("%+v %v", out, err)
	}
}
