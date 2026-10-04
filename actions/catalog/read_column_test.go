package catalog

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type columnListReaderStub struct {
	calls int
	pages []value.MetadataPage[value.MetadataColumn]
}

func (r *columnListReaderStub) DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.calls++
	return r.pages[r.calls-1], nil
}
func TestColumnListValidationBeforeRead(t *testing.T) {
	r := &columnListReaderStub{}
	_, err := columnListService(r).ListCatalogColumns(context.Background(), ColumnListInput{Limit: -1})
	if err == nil || r.calls != 0 {
		t.Fatal("invalid bound reached reader")
	}
}
func TestColumnListBoundedPagesAndProjection(t *testing.T) {
	r := &columnListReaderStub{pages: []value.MetadataPage[value.MetadataColumn]{{Items: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{LUID: "one", Name: "One", Type: "column"}}}, NextCursor: "opaque", Total: 2}, {Items: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{LUID: "two", Name: "Two", Type: "column"}}}, Complete: true, Total: 2}}}
	out, err := columnListService(r).ListCatalogColumns(context.Background(), ColumnListInput{All: true, TableID: "parent"})
	if err != nil || len(out.Items) != 2 || !out.Complete {
		t.Fatalf("%+v %v", out, err)
	}
	out.CompactOutput()
	out.FullOutput()
	if r.calls != 2 {
		t.Fatal("projection fetched remotely")
	}
}
func TestColumnListRepeatedCursorFails(t *testing.T) {
	r := &columnListReaderStub{pages: []value.MetadataPage[value.MetadataColumn]{{NextCursor: "x"}, {NextCursor: "x"}}}
	_, err := columnListService(r).ListCatalogColumns(context.Background(), ColumnListInput{All: true, TableID: "parent"})
	if err == nil {
		t.Fatal("repeated cursor accepted")
	}
}

type columnInspectReaderStub struct {
	calls int
	item  value.MetadataColumn
}

func (r *columnInspectReaderStub) GetColumn(context.Context, string, string) (value.MetadataColumn, error) {
	r.calls++
	return r.item, nil
}
func (r *columnInspectReaderStub) DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.calls++
	return value.MetadataPage[value.MetadataColumn]{Items: []value.MetadataColumn{r.item}, Complete: true}, nil
}
func TestColumnInspectExactSelectorBeforeRead(t *testing.T) {
	r := &columnInspectReaderStub{}
	_, e := columnInspectService(r).InspectCatalogColumn(context.Background(), ColumnInspectInput{ID: "x", MetadataID: "y"})
	if e == nil || r.calls != 0 {
		t.Fatal("conflicting selectors reached provider")
	}
}
func TestColumnInspectRejectWrongIdentity(t *testing.T) {
	r := &columnInspectReaderStub{item: value.MetadataColumn{MetadataIdentity: value.MetadataIdentity{LUID: "wrong"}}}
	_, e := columnInspectService(r).InspectCatalogColumn(context.Background(), ColumnInspectInput{ID: "expected", TableID: "table"})
	if e == nil {
		t.Fatal("mismatched identity accepted")
	}
}
func TestColumnInspectReadMetadataIdentity(t *testing.T) {
	r := &columnInspectReaderStub{item: value.MetadataColumn{MetadataIdentity: value.MetadataIdentity{MetadataID: "meta", Name: "fixture"}}}
	o, e := columnInspectService(r).InspectCatalogColumn(context.Background(), ColumnInspectInput{MetadataID: "meta"})
	if e != nil {
		t.Fatal(e)
	}
	o.CompactOutput()
	o.FullOutput()
	if r.calls != 1 {
		t.Fatal("output fetched again")
	}
}
