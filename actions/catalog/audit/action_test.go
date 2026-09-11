package audit

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
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
	o, e := New(r).Execute(context.Background(), Input{Type: "database", ID: "db"})
	if e != nil || o.Summary.Missing != 0 || o.Summary.Unknown != 2 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestObservedEmptyIsMissing(t *testing.T) {
	r := &reader{}
	o, e := New(r).Execute(context.Background(), Input{Type: "table", ID: "table"})
	if e != nil || o.Summary.Missing != 2 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestInheritedDescriptionPolicy(t *testing.T) {
	r := &reader{}
	in := Input{Type: "datasource", ID: "ds", Checks: []string{"descriptions"}}
	o, e := New(r).Execute(context.Background(), in)
	if e != nil || o.Summary.Present != 1 {
		t.Fatalf("%+v %v", o, e)
	}
	in.DirectOnly = true
	o, e = New(r).Execute(context.Background(), in)
	if e != nil || o.Summary.Missing != 1 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestNoUnscopedAudit(t *testing.T) {
	r := &reader{}
	_, e := New(r).Execute(context.Background(), Input{Type: "database"})
	if e == nil || r.calls != 0 {
		t.Fatal("unbounded scope accepted")
	}
}
