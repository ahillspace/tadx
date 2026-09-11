package inspect

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type reader struct {
	calls int
	item  value.MetadataDatabase
}

func (r *reader) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	r.calls++
	return r.item, nil
}
func (r *reader) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	r.calls++
	return value.MetadataPage[value.MetadataDatabase]{Items: []value.MetadataDatabase{r.item}, Complete: true}, nil
}
func TestExactSelectorBeforeRead(t *testing.T) {
	r := &reader{}
	_, e := New(r).Execute(context.Background(), Input{ID: "x", MetadataID: "y"})
	if e == nil || r.calls != 0 {
		t.Fatal("conflicting selectors reached provider")
	}
}
func TestRejectWrongIdentity(t *testing.T) {
	r := &reader{item: value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "wrong"}}}
	_, e := New(r).Execute(context.Background(), Input{ID: "expected"})
	if e == nil {
		t.Fatal("mismatched identity accepted")
	}
}
func TestReadMetadataIdentity(t *testing.T) {
	r := &reader{item: value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{MetadataID: "meta", Name: "fixture"}}}
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
