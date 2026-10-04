package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type databaseListReaderStub struct {
	calls int
	pages []value.MetadataPage[value.MetadataDatabase]
}

func (r *databaseListReaderStub) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	r.calls++
	return r.pages[r.calls-1], nil
}
func TestDatabaseListValidationBeforeRead(t *testing.T) {
	r := &databaseListReaderStub{}
	_, err := databaseListService(r).ListCatalogDatabases(context.Background(), DatabaseListInput{Limit: -1})
	if err == nil || r.calls != 0 {
		t.Fatal("invalid bound reached reader")
	}
}
func TestDatabaseListBoundedPagesAndProjection(t *testing.T) {
	r := &databaseListReaderStub{pages: []value.MetadataPage[value.MetadataDatabase]{{Items: []value.MetadataDatabase{{MetadataIdentity: value.MetadataIdentity{LUID: "one", Name: "One", Type: "database"}}}, NextCursor: "opaque", Total: 2}, {Items: []value.MetadataDatabase{{MetadataIdentity: value.MetadataIdentity{LUID: "two", Name: "Two", Type: "database"}}}, Complete: true, Total: 2}}}
	out, err := databaseListService(r).ListCatalogDatabases(context.Background(), DatabaseListInput{All: true})
	if err != nil || len(out.Items) != 2 || !out.Complete {
		t.Fatalf("%+v %v", out, err)
	}
	out.CompactOutput()
	out.FullOutput()
	if r.calls != 2 {
		t.Fatal("projection fetched remotely")
	}
}
func TestDatabaseListRepeatedCursorFails(t *testing.T) {
	r := &databaseListReaderStub{pages: []value.MetadataPage[value.MetadataDatabase]{{NextCursor: "x"}, {NextCursor: "x"}}}
	_, err := databaseListService(r).ListCatalogDatabases(context.Background(), DatabaseListInput{All: true})
	if err == nil {
		t.Fatal("repeated cursor accepted")
	}
}

type databaseInspectReaderStub struct {
	calls int
	item  value.MetadataDatabase
}

func (r *databaseInspectReaderStub) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	r.calls++
	return r.item, nil
}
func (r *databaseInspectReaderStub) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	r.calls++
	return value.MetadataPage[value.MetadataDatabase]{Items: []value.MetadataDatabase{r.item}, Complete: true}, nil
}
func TestDatabaseInspectExactSelectorBeforeRead(t *testing.T) {
	r := &databaseInspectReaderStub{}
	_, e := databaseInspectService(r).InspectCatalogDatabase(context.Background(), DatabaseInspectInput{ID: "x", MetadataID: "y"})
	if e == nil || r.calls != 0 {
		t.Fatal("conflicting selectors reached provider")
	}
}
func TestDatabaseInspectRejectWrongIdentity(t *testing.T) {
	r := &databaseInspectReaderStub{item: value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "wrong"}}}
	_, e := databaseInspectService(r).InspectCatalogDatabase(context.Background(), DatabaseInspectInput{ID: "expected"})
	if e == nil {
		t.Fatal("mismatched identity accepted")
	}
}
func TestDatabaseInspectReadMetadataIdentity(t *testing.T) {
	r := &databaseInspectReaderStub{item: value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{MetadataID: "meta", Name: "fixture"}}}
	o, e := databaseInspectService(r).InspectCatalogDatabase(context.Background(), DatabaseInspectInput{MetadataID: "meta"})
	if e != nil {
		t.Fatal(e)
	}
	o.CompactOutput()
	o.FullOutput()
	if r.calls != 1 {
		t.Fatal("output fetched again")
	}
}

type explorationPartialDatabaseReader struct{}

func (explorationPartialDatabaseReader) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	return value.MetadataDatabase{}, errors.New("unavailable")
}

func (explorationPartialDatabaseReader) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	return value.MetadataPage[value.MetadataDatabase]{Items: []value.MetadataDatabase{{MetadataIdentity: value.MetadataIdentity{MetadataID: "metadata-db", Name: "Confirmed"}}}}, errors.New("requested coverage unavailable")
}

func TestDatabaseInspectExplorationCatalogConfirmedItemRemainsPartial(t *testing.T) {
	out, err := databaseInspectService(explorationPartialDatabaseReader{}).InspectCatalogDatabase(t.Context(), DatabaseInspectInput{MetadataID: "metadata-db"})
	if err == nil || out.Status != "partial" || out.Item == nil || out.Item.Name != "Confirmed" {
		t.Fatalf("partial inspection evidence: %#v, %v", out, err)
	}
	compactDatabaseInspect := out.CompactOutput()
	if compactDatabaseInspect == nil || out.FullOutput() == nil {
		t.Fatal("partial inspection projection disappeared")
	}
}
