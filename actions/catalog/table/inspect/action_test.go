package inspect

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type reader struct {
	calls int
	item  value.MetadataTable
}

func (r *reader) GetTable(context.Context, string) (value.MetadataTable, error) {
	r.calls++
	return r.item, nil
}
func (r *reader) DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.calls++
	return value.MetadataPage[value.MetadataTable]{Items: []value.MetadataTable{r.item}, Complete: true}, nil
}
func TestExactSelectorBeforeRead(t *testing.T) {
	r := &reader{}
	_, e := New(r).Execute(context.Background(), Input{ID: "x", MetadataID: "y"})
	if e == nil || r.calls != 0 {
		t.Fatal("conflicting selectors reached provider")
	}
}
func TestRejectWrongIdentity(t *testing.T) {
	r := &reader{item: value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "wrong"}}}
	_, e := New(r).Execute(context.Background(), Input{ID: "expected"})
	if e == nil {
		t.Fatal("mismatched identity accepted")
	}
}
func TestReadMetadataIdentity(t *testing.T) {
	r := &reader{item: value.MetadataTable{MetadataIdentity: value.MetadataIdentity{MetadataID: "meta", Name: "fixture"}}}
	o, e := New(r).Execute(context.Background(), Input{MetadataID: "meta"})
	if e != nil {
		t.Fatal(e)
	}
	o.CompactOutput()
	o.FullOutput()
	if r.calls != 1 {
		t.Fatal("output fetched again")
	}
}
