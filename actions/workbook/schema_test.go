package workbook

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type schemaReader struct {
	data value.WorkbookSchema
	err  error
}

func filteredSchemaFixture() schemaReader {
	return schemaReader{data: value.WorkbookSchema{
		Metadata: value.MetadataWorkbookDatasources{
			Identity: value.MetadataIdentity{LUID: "wb", MetadataID: "mw"}, Complete: true,
			Datasources: []value.MetadataWorkbookDatasource{
				{Identity: value.MetadataIdentity{MetadataID: "a", Name: "Local"}, Kind: "embedded", Complete: true},
				{Identity: value.MetadataIdentity{MetadataID: "b", Name: "Published"}, Kind: "published", Complete: true},
			},
		},
		Fields: [][]value.SchemaField{
			{{ID: "Revenue", Caption: "Local sales", Role: "measure"}, {ID: "Region", Caption: "Region", Role: "dimension"}},
			{{ID: "Revenue", Caption: "Published sales", Role: "measure"}, {ID: "Margin", Caption: "Profit", Role: "measure", Formula: "[Revenue] - [Cost]"}},
		},
	}}
}

func TestSchemaFiltersSelectExactObjectsAcrossGroups(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input SchemaInput
		want  []string
	}{
		{"caption and role", SchemaInput{Query: " SALES ", Role: " MEASURE "}, []string{"a/Revenue", "b/Revenue"}},
		{"formula", SchemaInput{Query: "cost"}, []string{"b/Margin"}},
		{"dimension", SchemaInput{Role: "dimension"}, []string{"a/Region"}},
		{"datasource and field", SchemaInput{Datasource: "Published", FieldIDs: []string{"Revenue"}}, []string{"b/Revenue"}},
		{"field disambiguated by text", SchemaInput{Query: "local", FieldIDs: []string{"Revenue"}}, []string{"a/Revenue"}},
		{"no matches", SchemaInput{Query: "absent"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.WorkbookLUID = "wb"
			out, err := Schema(t.Context(), filteredSchemaFixture(), tc.input)
			if err != nil || !out.Complete || !out.Page.TotalComplete || out.Page.Total != len(tc.want) {
				t.Fatalf("filtered coverage: %+v, %v", out, err)
			}
			var got []string
			for _, group := range out.Datasources {
				for _, field := range group.Fields {
					got = append(got, group.Identity.MetadataID+"/"+field.ID)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("selected identities = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSchemaRejectsAmbiguousOrUnmatchedSelectors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     SchemaInput
		duplicate bool
	}{
		{"ambiguous field", SchemaInput{FieldIDs: []string{"Revenue"}}, false},
		{"missing field", SchemaInput{FieldIDs: []string{"Unknown"}}, false},
		{"field excluded by filter", SchemaInput{FieldIDs: []string{"Revenue"}, Role: "dimension"}, false},
		{"datasource name is exact", SchemaInput{Datasource: "local"}, false},
		{"duplicate datasource names", SchemaInput{Datasource: "Local"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := filteredSchemaFixture()
			if tc.duplicate {
				r.data.Metadata.Datasources[1].Identity.Name = "Local"
			}
			tc.input.WorkbookLUID = "wb"
			_, err := Schema(t.Context(), r, tc.input)
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.Kind != errs.KindUsage {
				t.Fatalf("selector error = %v, want usage failure", err)
			}
		})
	}
}

func TestSchemaMetadataFlagsPreserveProviderObservation(t *testing.T) {
	r := filteredSchemaFixture()
	metadata := value.FieldDescription{
		MetadataID: "field-meta", Description: new("Field meaning"), InheritedObserved: true,
		Inherited:       []value.DescriptionObservation{{Value: new("Inherited meaning"), SourceMetadataID: "column-meta"}},
		UpstreamColumns: []value.MetadataColumn{{MetadataIdentity: value.MetadataIdentity{MetadataID: "column-meta", Name: "Sales"}, Description: new("Column meaning"), Tags: []string{"reviewed"}, TagsObserved: true}},
	}
	r.data.Fields[0][0].Metadata = &metadata
	for _, flags := range []struct{ descriptions, tags bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		out, err := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Datasource: "Local", FieldIDs: []string{"Revenue"}, Descriptions: flags.descriptions, Tags: flags.tags})
		if err != nil {
			t.Fatal(err)
		}
		got := out.Datasources[0].Fields[0].Metadata
		if got == nil || got.MetadataID != "field-meta" || len(got.UpstreamColumns) != 1 {
			t.Fatalf("provenance lost: %+v", got)
		}
		column := got.UpstreamColumns[0]
		if (got.Description != nil) != flags.descriptions || (len(got.Inherited) > 0) != flags.descriptions || got.InheritedObserved != flags.descriptions || (column.Description != nil) != flags.descriptions {
			t.Fatalf("description flags %+v: %+v", flags, got)
		}
		if (len(column.Tags) > 0) != flags.tags || column.TagsObserved != flags.tags {
			t.Fatalf("tag flags %+v: %+v", flags, column)
		}
		if !reflect.DeepEqual(*r.data.Fields[0][0].Metadata, metadata) {
			t.Fatal("schema presentation mutated the provider observation")
		}
	}
}

func TestSchemaCursorRejectsChangedRequest(t *testing.T) {
	r := filteredSchemaFixture()
	first, err := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Environment: "test", Site: "site", Limit: 1})
	if err != nil || first.Page.NextCursor == "" {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	for _, change := range []func(*SchemaInput){
		func(in *SchemaInput) { in.Query = "sales" },
		func(in *SchemaInput) { in.Role = "measure" },
		func(in *SchemaInput) { in.Datasource = "Local" },
		func(in *SchemaInput) { in.FieldIDs = []string{"Region"} },
		func(in *SchemaInput) { in.Environment = "other" },
		func(in *SchemaInput) { in.Site = "other" },
		func(in *SchemaInput) { in.Descriptions = true },
		func(in *SchemaInput) { in.Tags = true },
	} {
		in := SchemaInput{WorkbookLUID: "wb", Environment: "test", Site: "site", Limit: 1, Cursor: first.Page.NextCursor}
		change(&in)
		_, err := Schema(t.Context(), r, in)
		failure, ok := errors.AsType[*errs.Error](err)
		if !ok || failure.Kind != errs.KindUsage {
			t.Fatalf("changed request accepted: %+v, %v", in, err)
		}
	}
	second, err := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Environment: "test", Site: "site", Limit: 2, Cursor: first.Page.NextCursor})
	if err != nil || second.Page.Returned != 2 || second.Page.Total != 4 {
		t.Fatalf("changing only page size failed: %+v, %v", second, err)
	}
}

func TestSchemaIncompleteGroupNeverClaimsCompleteTotals(t *testing.T) {
	r := filteredSchemaFixture()
	r.data.Metadata.Datasources[1].Complete = false
	out, err := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", All: true})
	if err != nil || out.Status != "partial" || out.Complete || out.Page.TotalComplete || out.Page.Returned != 4 {
		t.Fatalf("incomplete group coverage overstated: %+v, %v", out, err)
	}
}

func TestSchemaPartialInventoryPreservesFilteredEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input SchemaInput
		count int
	}{
		{"unknown field absence", SchemaInput{Datasource: "Local", FieldIDs: []string{"Revenue", "Absent"}}, 1},
		{"unknown field uniqueness", SchemaInput{Datasource: "Local", FieldIDs: []string{"Revenue"}}, 1},
		{"unobserved datasource", SchemaInput{Datasource: "Unobserved"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := filteredSchemaFixture()
			r.data.Metadata.Complete = false
			r.err = errors.New("later inventory unavailable")
			tc.input.WorkbookLUID = "wb"
			out, err := Schema(t.Context(), r, tc.input)
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.Kind != errs.KindOperation || !errors.Is(err, r.err) || out.Status != "partial" || out.Complete || out.Page.TotalComplete || out.Page.Returned != tc.count {
				t.Fatalf("partial filtered evidence: %+v, %v", out, err)
			}
		})
	}
	r := filteredSchemaFixture()
	r.data.Metadata.Complete = false
	_, err := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", FieldIDs: []string{"Revenue"}})
	failure, ok := errors.AsType[*errs.Error](err)
	if !ok || failure.Kind != errs.KindUsage {
		t.Fatalf("observed duplicate field did not fail ambiguity: %v", err)
	}
}

func TestSchemaReportsPartialTotals(t *testing.T) {
	r := schemaReader{data: value.WorkbookSchema{
		Metadata: value.MetadataWorkbookDatasources{Identity: value.MetadataIdentity{LUID: "wb", MetadataID: "mw"}, Datasources: []value.MetadataWorkbookDatasource{{Identity: value.MetadataIdentity{MetadataID: "ds", Name: "Local"}, Kind: "embedded"}}},
		Fields:   [][]value.SchemaField{{{ID: "Revenue", Caption: "Revenue"}}},
	}, err: errors.New("next page unavailable")}
	out, err := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb"})
	if err == nil || out.Status != "partial" || out.Complete || out.Page.TotalComplete || out.Page.Returned != 1 {
		t.Fatalf("partial evidence lost or overstated: %+v, %v", out, err)
	}
}

func (r schemaReader) ReadWorkbookSchema(context.Context, string) (value.WorkbookSchema, error) {
	return r.data, r.err
}

func TestSchemaGroupsAndGlobalPaging(t *testing.T) {
	r := schemaReader{data: value.WorkbookSchema{
		Metadata: value.MetadataWorkbookDatasources{Identity: value.MetadataIdentity{LUID: "wb", MetadataID: "mw", Name: "Book"}, Complete: true,
			Datasources: []value.MetadataWorkbookDatasource{{Identity: value.MetadataIdentity{MetadataID: "a", Name: "Local"}, Kind: "embedded", Complete: true}, {Identity: value.MetadataIdentity{MetadataID: "b", LUID: "ds", Name: "Published"}, Kind: "published", Complete: true}}},
		Fields: [][]value.SchemaField{{{ID: "Revenue", Caption: "Revenue", Role: "measure"}}, {{ID: "Revenue", Caption: "Revenue", Role: "measure"}}},
	}}
	o, err := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Limit: 1})
	if err != nil || o.Page.Total != 2 || o.Page.Returned != 1 || o.Page.NextCursor == "" {
		t.Fatalf("%+v %v", o, err)
	}
	_, err = Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", FieldIDs: []string{"Revenue"}})
	if err == nil {
		t.Fatal("ambiguous field accepted")
	}
	o, err = Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Datasource: "Local", FieldIDs: []string{"Revenue"}})
	if err != nil || o.Page.Total != 1 {
		t.Fatalf("scoped selection: %+v %v", o, err)
	}
	first, _ := Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Limit: 1})
	o, err = Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || o.Datasources[0].Kind != "published" {
		t.Fatalf("continuation: %+v %v", o, err)
	}
	r.data.Fields[0][0].Caption = "Changed"
	if _, err = Schema(t.Context(), r, SchemaInput{WorkbookLUID: "wb", Limit: 1, Cursor: first.Page.NextCursor}); err == nil {
		t.Fatal("changed schema accepted old cursor")
	}
}
