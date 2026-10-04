package catalog

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

func TestNilCatalogReaders(t *testing.T) {
	cases := []struct {
		operation string
		run       func() error
	}{
		{"catalog.database.list", func() error {
			_, err := databaseListService(nil).ListCatalogDatabases(t.Context(), DatabaseListInput{})
			return err
		}},
		{"catalog.database.inspect", func() error {
			_, err := databaseInspectService(nil).InspectCatalogDatabase(t.Context(), DatabaseInspectInput{ID: "rest"})
			return err
		}},
		{"catalog.table.list", func() error {
			_, err := tableListService(nil).ListCatalogTables(t.Context(), TableListInput{})
			return err
		}},
		{"catalog.table.inspect", func() error {
			_, err := tableInspectService(nil).InspectCatalogTable(t.Context(), TableInspectInput{ID: "rest"})
			return err
		}},
		{"catalog.column.list", func() error {
			_, err := columnListService(nil).ListCatalogColumns(t.Context(), ColumnListInput{TableID: "parent"})
			return err
		}},
		{"catalog.column.inspect", func() error {
			_, err := columnInspectService(nil).InspectCatalogColumn(t.Context(), ColumnInspectInput{ID: "rest", TableID: "parent"})
			return err
		}},
		{"catalog.search", func() error {
			_, err := searchService(nil).SearchCatalog(t.Context(), SearchInput{Query: "match"})
			return err
		}},
		{"catalog.audit", func() error {
			_, err := auditService(nil).AuditCatalog(t.Context(), AuditInput{Type: "database", ID: "rest"})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.operation, func(t *testing.T) {
			err := tc.run()
			structured, ok := errors.AsType[*errs.Error](err)
			if !ok || structured.ID != tc.operation+".usage" || structured.Kind != errs.KindUsage {
				t.Fatalf("nil reader error = %v", err)
			}
		})
	}
}

type databasePages struct {
	queries []value.MetadataQuery
	pages   []value.MetadataPage[value.MetadataDatabase]
	fail    bool
}

func (r *databasePages) DiscoverDatabases(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	r.queries = append(r.queries, q)
	if r.fail && len(r.queries) > 1 {
		return value.MetadataPage[value.MetadataDatabase]{}, errors.New("later page unavailable")
	}
	return r.pages[len(r.queries)-1], nil
}
func TestDatabaseListBoundsAndPartialEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		all   bool
		want  int
	}{
		{"default", 0, false, 25}, {"all", 0, true, 10000}, {"explicit", 3, false, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &databasePages{pages: []value.MetadataPage[value.MetadataDatabase]{{Complete: true}}}
			out, err := databaseListService(r).ListCatalogDatabases(t.Context(), DatabaseListInput{Limit: tc.limit, All: tc.all})
			if err != nil || out.Page.Limit != tc.want || len(r.queries) != 1 || r.queries[0].Limit != min(100, tc.want) || !out.Complete {
				t.Fatalf("bounds: %+v %+v %v", out, r.queries, err)
			}
		})
	}
	r := &databasePages{fail: true, pages: []value.MetadataPage[value.MetadataDatabase]{{Items: []value.MetadataDatabase{{MetadataIdentity: value.MetadataIdentity{LUID: "rest", MetadataID: "meta"}}}, NextCursor: "next", Total: 2, ObservedAt: "observed", TableauRequestID: "request"}}}
	out, err := databaseListService(r).ListCatalogDatabases(t.Context(), DatabaseListInput{All: true})
	if err == nil || out.Status != "partial" || out.Complete || !out.Page.MoreAvailable || out.Page.Returned != 1 || len(out.Items) != 1 || out.Items[0].MetadataID != "meta" || out.ObservedAt != "observed" || out.RequestID != "request" || len(r.queries) != 2 || r.queries[1].Cursor != "next" {
		t.Fatalf("partial evidence: %+v %+v %v", out, r.queries, err)
	}
}

type tablePages struct {
	queries []value.MetadataQuery
	pages   []value.MetadataPage[value.MetadataTable]
	fail    bool
}

func (r *tablePages) DiscoverTables(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.queries = append(r.queries, q)
	if r.fail && len(r.queries) > 1 {
		return value.MetadataPage[value.MetadataTable]{}, errors.New("later page unavailable")
	}
	return r.pages[len(r.queries)-1], nil
}
func TestTableListBoundsAndPartialEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		all   bool
		want  int
	}{
		{"default", 0, false, 25}, {"all", 0, true, 10000}, {"explicit", 3, false, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &tablePages{pages: []value.MetadataPage[value.MetadataTable]{{Complete: true}}}
			out, err := tableListService(r).ListCatalogTables(t.Context(), TableListInput{DatabaseID: "parent", Limit: tc.limit, All: tc.all})
			if err != nil || out.Page.Limit != tc.want || len(r.queries) != 1 || r.queries[0].Limit != min(100, tc.want) || !out.Complete {
				t.Fatalf("bounds: %+v %+v %v", out, r.queries, err)
			}
		})
	}
	r := &tablePages{fail: true, pages: []value.MetadataPage[value.MetadataTable]{{Items: []value.MetadataTable{{MetadataIdentity: value.MetadataIdentity{LUID: "rest", MetadataID: "meta"}}}, NextCursor: "next", Total: 2, ObservedAt: "observed", TableauRequestID: "request"}}}
	out, err := tableListService(r).ListCatalogTables(t.Context(), TableListInput{DatabaseID: "parent", All: true})
	if err == nil || out.Status != "partial" || out.Complete || !out.Page.MoreAvailable || out.Page.Returned != 1 || len(out.Items) != 1 || out.Items[0].MetadataID != "meta" || out.ObservedAt != "observed" || out.RequestID != "request" || len(r.queries) != 2 || r.queries[1].Cursor != "next" {
		t.Fatalf("partial evidence: %+v %+v %v", out, r.queries, err)
	}
}

type columnPages struct {
	queries []value.MetadataQuery
	pages   []value.MetadataPage[value.MetadataColumn]
	fail    bool
}

func (r *columnPages) DiscoverColumns(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.queries = append(r.queries, q)
	if r.fail && len(r.queries) > 1 {
		return value.MetadataPage[value.MetadataColumn]{}, errors.New("later page unavailable")
	}
	return r.pages[len(r.queries)-1], nil
}
func TestColumnListBoundsAndPartialEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		all   bool
		want  int
	}{
		{"default", 0, false, 25}, {"all", 0, true, 10000}, {"explicit", 3, false, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &columnPages{pages: []value.MetadataPage[value.MetadataColumn]{{Complete: true}}}
			out, err := columnListService(r).ListCatalogColumns(t.Context(), ColumnListInput{TableID: "parent", Limit: tc.limit, All: tc.all})
			if err != nil || out.Page.Limit != tc.want || len(r.queries) != 1 || r.queries[0].Limit != min(100, tc.want) || !out.Complete {
				t.Fatalf("bounds: %+v %+v %v", out, r.queries, err)
			}
		})
	}
	r := &columnPages{fail: true, pages: []value.MetadataPage[value.MetadataColumn]{{Items: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{LUID: "rest", MetadataID: "meta"}}}, NextCursor: "next", Total: 2, ObservedAt: "observed", TableauRequestID: "request"}}}
	out, err := columnListService(r).ListCatalogColumns(t.Context(), ColumnListInput{TableID: "parent", All: true})
	if err == nil || out.Status != "partial" || out.Complete || !out.Page.MoreAvailable || out.Page.Returned != 1 || len(out.Items) != 1 || out.Items[0].MetadataID != "meta" || out.ObservedAt != "observed" || out.RequestID != "request" || len(r.queries) != 2 || r.queries[1].Cursor != "next" {
		t.Fatalf("partial evidence: %+v %+v %v", out, r.queries, err)
	}
}
