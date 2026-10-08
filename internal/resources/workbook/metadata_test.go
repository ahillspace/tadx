package workbook_test

import (
	"context"
	"errors"
	"testing"

	resource "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
	"github.com/ahillspace/tadx/internal/value"
)

type metadataIdentityClient struct {
	result tableauworkbook.Workbook
	err    error
	calls  int
}

func (c *metadataIdentityClient) Get(_ context.Context, _ string) (tableauworkbook.Workbook, error) {
	c.calls++
	return c.result, c.err
}

type workbookMetadataReader struct {
	result value.MetadataWorkbookDatasources
	err    error
	calls  int
}

func (c *workbookMetadataReader) WorkbookDatasources(_ context.Context, _ string) (value.MetadataWorkbookDatasources, error) {
	c.calls++
	return c.result, c.err
}

func TestWorkbookMetadataAdapterPreservesLocalFieldAndResolvesPublishedShape(t *testing.T) {
	identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "wb-1", Name: "Book"}}
	localDescription := "Local meaning"
	metadata := &workbookMetadataReader{result: value.MetadataWorkbookDatasources{
		Identity: value.MetadataIdentity{LUID: "wb-1", MetadataID: "wb-meta", Name: "Book"}, Complete: true,
		Datasources: []value.MetadataWorkbookDatasource{
			{Identity: value.MetadataIdentity{MetadataID: "embedded-meta", Name: "Local"}, Kind: "embedded", Fields: []value.FieldDescription{{MetadataID: "local-meta", Name: "Local Sales", FullyQualifiedName: "[Sales]", Description: &localDescription, RemoteField: &value.MetadataIdentity{MetadataID: "remote-meta"}}}},
			{Identity: value.MetadataIdentity{MetadataID: "published-meta", LUID: "ds-1", Name: "Published"}, Kind: "published", Fields: []value.FieldDescription{{MetadataID: "remote-meta", Name: "Sales", FullyQualifiedName: "[Sales]", Type: "ColumnField", Role: "MEASURE", DataType: "REAL", Aggregation: "SUM"}}},
		},
	}}
	got, err := resource.NewMetadataAdapter(identity, metadata).ReadWorkbookSchema(t.Context(), "wb-1")
	if err != nil || len(got.Fields) != 2 || len(got.Fields[0]) != 1 {
		t.Fatalf("got %+v, error %v", got, err)
	}
	f := got.Fields[0][0]
	if f.Role != "measure" || f.DataType != "REAL" || f.DefaultAggregation != "SUM" || f.Caption != "Local Sales" || f.Metadata == nil || f.Metadata.Description == nil || *f.Metadata.Description != localDescription {
		t.Fatalf("field %+v", f)
	}
}

func TestWorkbookMetadataAdapterPreservesUnknownAndPartialObservation(t *testing.T) {
	identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "wb-1", Name: "Book"}}
	metadata := &workbookMetadataReader{result: value.MetadataWorkbookDatasources{
		Identity:    value.MetadataIdentity{LUID: "wb-1", MetadataID: "wb-meta", Name: "Book"},
		Datasources: []value.MetadataWorkbookDatasource{{Identity: value.MetadataIdentity{MetadataID: "emb-meta", Name: "Local"}, Kind: "embedded", Fields: []value.FieldDescription{{MetadataID: "field-meta", Name: "Opaque", FullyQualifiedName: "[Opaque]"}}}},
	}, err: errors.New("later page failed")}
	got, err := resource.NewMetadataAdapter(identity, metadata).ReadWorkbookSchema(t.Context(), "wb-1")
	if err == nil || len(got.Fields) != 1 || len(got.Fields[0]) != 1 || got.Fields[0][0].Role != "" || got.Fields[0][0].DataType != "" {
		t.Fatalf("got %+v, error %v", got, err)
	}
}

func TestWorkbookMetadataAdapterChecksRESTIdentityFirst(t *testing.T) {
	identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "other", Name: "Book"}}
	metadata := &workbookMetadataReader{}
	_, err := resource.NewMetadataAdapter(identity, metadata).ReadWorkbookSchema(t.Context(), "wb-1")
	if err == nil || metadata.calls != 0 {
		t.Fatalf("metadata calls %d, error %v", metadata.calls, err)
	}
}
