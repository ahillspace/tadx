package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type reader struct{ calls int }

type workbookReader struct {
	reader
	result value.MetadataWorkbookDatasources
	err    error
}

func (r *workbookReader) WorkbookFieldDescriptions(context.Context, string) (value.MetadataWorkbookDatasources, error) {
	return r.result, r.err
}

func TestWorkbookAuditSeparatesLocalAndPublishedFindings(t *testing.T) {
	r := &workbookReader{result: value.MetadataWorkbookDatasources{
		Identity:    value.MetadataIdentity{LUID: "wb", MetadataID: "mw", Name: "Workbook", Type: "Workbook"},
		Description: new("Workbook purpose"), TagsObserved: true, Complete: true,
		Datasources: []value.MetadataWorkbookDatasource{
			{Identity: value.MetadataIdentity{MetadataID: "embedded", Name: "Local"}, Kind: "embedded", Complete: true,
				Fields: []value.FieldDescription{{MetadataID: "local-field", Name: "Revenue", Description: new(""), InheritedObserved: true, Inherited: []value.DescriptionObservation{{Value: new("Source meaning")}}}}},
			{Identity: value.MetadataIdentity{MetadataID: "published", LUID: "ds", Name: "Source", Type: "PublishedDatasource"}, Kind: "published", Description: new("Source purpose"), TagsObserved: true, Complete: true,
				Fields: []value.FieldDescription{{MetadataID: "source-field", Name: "Revenue", Description: new("Source meaning"), InheritedObserved: true}}},
		},
	}}
	out, err := Execute(t.Context(), r, Input{Type: "workbook", ID: "wb", Checks: []string{"descriptions"}})
	if err != nil || !out.Complete || out.Scanned != 4 || out.Summary.Present != 4 || len(out.Groups) != 3 {
		t.Fatalf("audit = %+v, err=%v", out, err)
	}
	if out.Findings[1].DatasourceMetadataID != "embedded" || out.Findings[1].Source != "inherited" {
		t.Fatalf("local provenance lost: %+v", out.Findings)
	}
	for i, kind := range []string{"workbook", "field", "datasource", "field"} {
		if out.Findings[i].Type != kind {
			t.Fatalf("finding %d type = %q, want %q", i, out.Findings[i].Type, kind)
		}
	}
	if out.Groups[0].Identity.Type != "Workbook" || out.Groups[2].Identity.Type != "PublishedDatasource" {
		t.Fatalf("raw provider identity types changed: %+v", out.Groups)
	}
	out, err = Execute(t.Context(), r, Input{Type: "workbook", ID: "wb", Checks: []string{"descriptions"}, DirectOnly: true, Limit: 2})
	if err != nil || out.Complete || out.Scanned != 2 || out.Summary.Missing != 1 {
		t.Fatalf("bounded direct-only audit = %+v, err=%v", out, err)
	}
	r.err = errors.New("later datasource read failed")
	out, err = Execute(t.Context(), r, Input{Type: "workbook", ID: "wb"})
	if err == nil || out.Complete || out.Scanned == 0 || out.Status != "partial" {
		t.Fatalf("confirmed evidence lost: %+v, %v", out, err)
	}
}

func TestWorkbookAuditTopologyAndObjectLimit(t *testing.T) {
	local := value.MetadataWorkbookDatasource{
		Identity: value.MetadataIdentity{MetadataID: "local", Name: "Local"}, Kind: "embedded", Complete: true,
		Fields: []value.FieldDescription{
			{MetadataID: "local-a", Name: "Revenue", Description: new("Local meaning"), InheritedObserved: true},
			{MetadataID: "local-b", Name: "Revenue alias", Description: new(""), InheritedObserved: true, Inherited: []value.DescriptionObservation{{Value: new("Source meaning"), SourceMetadataID: "source-field"}}},
		},
	}
	published := value.MetadataWorkbookDatasource{
		Identity: value.MetadataIdentity{MetadataID: "published", LUID: "ds", Name: "Source", Type: "datasource"}, Kind: "published", Complete: true,
		Description: new("Source purpose"), Tags: []string{"reviewed"}, TagsObserved: true,
		Fields: []value.FieldDescription{{MetadataID: "source-field", Name: "Revenue", Description: new("Source meaning"), InheritedObserved: true}},
	}
	for _, tc := range []struct {
		name    string
		groups  []value.MetadataWorkbookDatasource
		scanned int
	}{
		{"embedded only", []value.MetadataWorkbookDatasource{local}, 3},
		{"published only", []value.MetadataWorkbookDatasource{published}, 3},
		{"mixed", []value.MetadataWorkbookDatasource{local, published}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &workbookReader{result: value.MetadataWorkbookDatasources{
				Identity:    value.MetadataIdentity{LUID: "wb", MetadataID: "mw", Name: "Workbook", Type: "workbook"},
				Description: new("Workbook purpose"), Tags: []string{"reviewed"}, TagsObserved: true, Complete: true, Datasources: tc.groups,
			}}
			out, err := Execute(t.Context(), r, Input{Type: "workbook", ID: "wb"})
			if err != nil || !out.Complete || out.Scanned != tc.scanned || len(out.Groups) != len(tc.groups)+1 || out.Summary.Missing != 0 || out.Summary.Unknown != 0 {
				t.Fatalf("topology coverage: %+v, %v", out, err)
			}
			for _, finding := range out.Findings {
				if finding.Type == "field" && finding.Check == "tags" {
					t.Fatalf("unsupported field tags assessed: %+v", finding)
				}
			}
			bounded, err := Execute(t.Context(), r, Input{Type: "workbook", ID: "wb", Limit: 2})
			if err != nil || bounded.Complete || bounded.Scanned != 2 || bounded.Status != "partial" || len(bounded.Findings) < 3 {
				t.Fatalf("assessment limit must count objects, not checks: %+v, %v", bounded, err)
			}
		})
	}
}

func TestWorkbookAuditKeepsUnknownFieldDescriptionsDistinctFromMissing(t *testing.T) {
	r := &workbookReader{result: value.MetadataWorkbookDatasources{
		Identity:    value.MetadataIdentity{LUID: "wb", MetadataID: "mw", Name: "Workbook", Type: "workbook"},
		Description: new("Purpose"), TagsObserved: true, Complete: false,
		Datasources: []value.MetadataWorkbookDatasource{{Identity: value.MetadataIdentity{MetadataID: "local", Name: "Local"}, Kind: "embedded", Complete: false,
			Fields: []value.FieldDescription{{MetadataID: "field", Name: "Unknown"}}}},
	}, err: errors.New("metadata permission failure")}
	out, err := Execute(t.Context(), r, Input{Type: "workbook", ID: "wb", Checks: []string{"descriptions"}})
	if err == nil || out.Status != "partial" || out.Complete || out.Scanned != 2 || out.Summary.Unknown != 1 || out.Summary.Missing != 0 {
		t.Fatalf("unknown evidence lost or labeled missing: %+v, %v", out, err)
	}
	if len(out.Groups) != 2 || out.Groups[1].Complete || out.Findings[1].DatasourceMetadataID != "local" {
		t.Fatalf("partial datasource provenance lost: %+v", out)
	}
}

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
