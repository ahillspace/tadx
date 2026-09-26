package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type reader struct{ calls int }

func (r *reader) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	r.calls++
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "db", Name: "DB"}}, nil
}
func (r *reader) GetTable(context.Context, string) (value.MetadataTable, error) {
	r.calls++
	s := ""
	return value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "table", Name: "Table"}, Description: &s, TagsObserved: true}, nil
}
func (r *reader) DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.calls++
	return value.MetadataPage[value.MetadataTable]{Complete: true}, nil
}
func (r *reader) DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.calls++
	return value.MetadataPage[value.MetadataColumn]{Complete: true}, nil
}
func (r *reader) DatasourceFieldDescriptions(context.Context, string) (value.MetadataDatasourceDescriptions, error) {
	r.calls++
	empty := ""
	inherited := "Upstream meaning"
	return value.MetadataDatasourceDescriptions{LUID: "ds", Complete: true, Fields: []value.FieldDescription{{MetadataID: "field", Name: "Field", Description: &empty, Inherited: []value.DescriptionObservation{{Value: &inherited}}}}}, nil
}
func TestUnknownIsNotMissing(t *testing.T) {
	r := &reader{}
	o, e := Execute(context.Background(), r, Input{Type: "database", ID: "db"})
	if e != nil || o.Summary.Missing != 0 || o.Summary.Unknown != 2 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestObservedEmptyIsMissing(t *testing.T) {
	r := &reader{}
	o, e := Execute(context.Background(), r, Input{Type: "table", ID: "table"})
	if e != nil || o.Summary.Missing != 2 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestInheritedDescriptionPolicy(t *testing.T) {
	r := &reader{}
	in := Input{Type: "datasource", ID: "ds", Checks: []string{"descriptions"}}
	o, e := Execute(context.Background(), r, in)
	if e != nil || o.Summary.Present != 1 {
		t.Fatalf("%+v %v", o, e)
	}
	in.DirectOnly = true
	o, e = Execute(context.Background(), r, in)
	if e != nil || o.Summary.Missing != 1 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestNoUnscopedAudit(t *testing.T) {
	r := &reader{}
	_, e := Execute(context.Background(), r, Input{Type: "database"})
	if e == nil || r.calls != 0 {
		t.Fatal("unbounded scope accepted")
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
			out, err := Execute(t.Context(), r, Input{Type: "table", ID: "table"})
			if err == nil || out.Status != "partial" || out.Complete || out.Scanned != 2 || len(out.Findings) != 4 || out.Findings[2].MetadataID != "meta" {
				t.Fatalf("partial=%+v error=%v", out, err)
			}
			if len(r.queries) != 2 || r.queries[1].Cursor != "next" || r.queries[1].ParentLUID != "table" {
				t.Fatalf("queries=%+v", r.queries)
			}
		})
	}
}
