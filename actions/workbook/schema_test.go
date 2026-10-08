package workbook

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type schemaReader struct {
	data value.WorkbookSchema
	err  error
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
