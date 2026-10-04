package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type auditReaderFixture struct{ calls int }

func (r *auditReaderFixture) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	r.calls++
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "db", Name: "DB"}}, nil
}
func (r *auditReaderFixture) GetTable(context.Context, string) (value.MetadataTable, error) {
	r.calls++
	s := ""
	return value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "table", Name: "Table"}, Description: &s, TagsObserved: true}, nil
}
func (r *auditReaderFixture) DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	r.calls++
	return value.MetadataPage[value.MetadataTable]{Complete: true}, nil
}
func (r *auditReaderFixture) DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	r.calls++
	return value.MetadataPage[value.MetadataColumn]{Complete: true}, nil
}
func (r *auditReaderFixture) DatasourceFieldDescriptions(context.Context, string) (value.MetadataDatasourceDescriptions, error) {
	r.calls++
	empty := ""
	inherited := "Upstream meaning"
	return value.MetadataDatasourceDescriptions{LUID: "ds", Complete: true, Fields: []value.FieldDescription{{MetadataID: "field", Name: "Field", Description: &empty, Inherited: []value.DescriptionObservation{{Value: &inherited}}}}}, nil
}
func TestAuditUnknownIsNotMissing(t *testing.T) {
	r := &auditReaderFixture{}
	o, e := auditService(r).AuditCatalog(context.Background(), AuditInput{Type: "database", ID: "db"})
	if e != nil || o.Summary.Missing != 0 || o.Summary.Unknown != 2 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestAuditObservedEmptyIsMissing(t *testing.T) {
	r := &auditReaderFixture{}
	o, e := auditService(r).AuditCatalog(context.Background(), AuditInput{Type: "table", ID: "table"})
	if e != nil || o.Summary.Missing != 2 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestAuditInheritedDescriptionPolicy(t *testing.T) {
	r := &auditReaderFixture{}
	in := AuditInput{Type: "datasource", ID: "ds", Checks: []string{"descriptions"}}
	o, e := auditService(r).AuditCatalog(context.Background(), in)
	if e != nil || o.Summary.Present != 1 {
		t.Fatalf("%+v %v", o, e)
	}
	in.DirectOnly = true
	o, e = auditService(r).AuditCatalog(context.Background(), in)
	if e != nil || o.Summary.Missing != 1 {
		t.Fatalf("%+v %v", o, e)
	}
}
func TestAuditNoUnscopedAudit(t *testing.T) {
	r := &auditReaderFixture{}
	_, e := auditService(r).AuditCatalog(context.Background(), AuditInput{Type: "database"})
	if e == nil || r.calls != 0 {
		t.Fatal("unbounded scope accepted")
	}
}

type auditPartialPages struct {
	auditReaderFixture
	queries []value.MetadataQuery
	mode    string
}

func (r *auditPartialPages) DiscoverColumns(_ context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
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
func TestAuditTraversalRetainsPartialEvidence(t *testing.T) {
	for _, mode := range []string{"error", "cursor", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			r := &auditPartialPages{mode: mode}
			out, err := auditService(r).AuditCatalog(t.Context(), AuditInput{Type: "table", ID: "table"})
			if err == nil || out.Status != "partial" || out.Complete || out.Scanned != 2 || len(out.Findings) != 4 || out.Findings[2].MetadataID != "meta" {
				t.Fatalf("partial=%+v error=%v", out, err)
			}
			if len(r.queries) != 2 || r.queries[1].Cursor != "next" || r.queries[1].ParentLUID != "table" {
				t.Fatalf("queries=%+v", r.queries)
			}
		})
	}
}
