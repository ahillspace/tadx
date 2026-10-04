package catalog

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type tableListReaderStub struct {
	calls int
	pages []value.MetadataPage[value.MetadataTable]
}

func (r *tableListReaderStub) DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.calls++
	return r.pages[r.calls-1], nil
}
func TestTableListValidationBeforeRead(t *testing.T) {
	r := &tableListReaderStub{}
	_, err := tableListService(r).ListCatalogTables(context.Background(), TableListInput{Limit: -1})
	if err == nil || r.calls != 0 {
		t.Fatal("invalid bound reached reader")
	}
}
func TestTableListBoundedPagesAndProjection(t *testing.T) {
	r := &tableListReaderStub{pages: []value.MetadataPage[value.MetadataTable]{{Items: []value.MetadataTable{{MetadataIdentity: value.MetadataIdentity{LUID: "one", Name: "One", Type: "table"}}}, NextCursor: "opaque", Total: 2}, {Items: []value.MetadataTable{{MetadataIdentity: value.MetadataIdentity{LUID: "two", Name: "Two", Type: "table"}}}, Complete: true, Total: 2}}}
	out, err := tableListService(r).ListCatalogTables(context.Background(), TableListInput{All: true, DatabaseID: "parent"})
	if err != nil || len(out.Items) != 2 || !out.Complete {
		t.Fatalf("%+v %v", out, err)
	}
	out.CompactOutput()
	out.FullOutput()
	if r.calls != 2 {
		t.Fatal("projection fetched remotely")
	}
}
func TestTableListRepeatedCursorFails(t *testing.T) {
	r := &tableListReaderStub{pages: []value.MetadataPage[value.MetadataTable]{{NextCursor: "x"}, {NextCursor: "x"}}}
	_, err := tableListService(r).ListCatalogTables(context.Background(), TableListInput{All: true, DatabaseID: "parent"})
	if err == nil {
		t.Fatal("repeated cursor accepted")
	}
}

type tableInspectReaderStub struct {
	calls int
	item  value.MetadataTable
}

func (r *tableInspectReaderStub) GetTable(context.Context, string) (value.MetadataTable, error) {
	r.calls++
	return r.item, nil
}
func (r *tableInspectReaderStub) DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.calls++
	return value.MetadataPage[value.MetadataTable]{Items: []value.MetadataTable{r.item}, Complete: true}, nil
}
func TestTableInspectExactSelectorBeforeRead(t *testing.T) {
	r := &tableInspectReaderStub{}
	_, e := tableInspectService(r).InspectCatalogTable(context.Background(), TableInspectInput{ID: "x", MetadataID: "y"})
	if e == nil || r.calls != 0 {
		t.Fatal("conflicting selectors reached provider")
	}
}
func TestTableInspectRejectWrongIdentity(t *testing.T) {
	r := &tableInspectReaderStub{item: value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "wrong"}}}
	_, e := tableInspectService(r).InspectCatalogTable(context.Background(), TableInspectInput{ID: "expected"})
	if e == nil {
		t.Fatal("mismatched identity accepted")
	}
}
func TestTableInspectReadMetadataIdentity(t *testing.T) {
	r := &tableInspectReaderStub{item: value.MetadataTable{MetadataIdentity: value.MetadataIdentity{MetadataID: "meta", Name: "fixture"}}}
	o, e := tableInspectService(r).InspectCatalogTable(context.Background(), TableInspectInput{MetadataID: "meta"})
	if e != nil {
		t.Fatal(e)
	}
	o.CompactOutput()
	o.FullOutput()
	if r.calls != 1 {
		t.Fatal("output fetched again")
	}
}
