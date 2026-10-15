package workbook_test

import (
	"context"
	"errors"
	"testing"

	resource "github.com/ahillspace/tadx/internal/resources/workbook"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
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

func (c *workbookMetadataReader) WorkbookFieldDescriptions(_ context.Context, _ string) (value.MetadataWorkbookDatasources, error) {
	c.calls++
	return c.result, c.err
}

type publishedSchemaReader struct {
	result fieldcatalog.Schema
	err    error
	calls  int
	luid   string
}

func (r *publishedSchemaReader) ReadDatasourceSchema(_ context.Context, luid string) (fieldcatalog.Schema, error) {
	r.calls++
	r.luid = luid
	return r.result, r.err
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
	published := &publishedSchemaReader{result: fieldcatalog.Schema{DatasourceLUID: "ds-1", Fields: []value.SchemaField{{ID: "Sales", Name: "Sales", Caption: "Sales", Role: "measure", DataType: "REAL", DefaultAggregation: "SUM", Table: "Orders", LogicalTableID: "orders"}}}}
	got, err := resource.NewMetadataAdapter(identity, metadata, published).ReadWorkbookSchema(t.Context(), "wb-1")
	if err != nil || len(got.Fields) != 2 || len(got.Fields[0]) != 1 {
		t.Fatalf("got %+v, error %v", got, err)
	}
	f := got.Fields[0][0]
	if f.Role != "measure" || f.DataType != "REAL" || f.DefaultAggregation != "SUM" || f.Caption != "Local Sales" || f.Metadata == nil || f.Metadata.Description == nil || *f.Metadata.Description != localDescription {
		t.Fatalf("field %+v", f)
	}
	if published.calls != 1 || published.luid != "ds-1" || got.Fields[1][0].Table != "Orders" || f.Table != "Orders" || f.Metadata.Role != "" {
		t.Fatalf("canonical schema or original local observation changed: %+v", got)
	}
}

func TestWorkbookMetadataAdapterPreservesUnknownAndPartialObservation(t *testing.T) {
	identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "wb-1", Name: "Book"}}
	metadata := &workbookMetadataReader{result: value.MetadataWorkbookDatasources{
		Identity:    value.MetadataIdentity{LUID: "wb-1", MetadataID: "wb-meta", Name: "Book"},
		Datasources: []value.MetadataWorkbookDatasource{{Identity: value.MetadataIdentity{MetadataID: "emb-meta", Name: "Local"}, Kind: "embedded", Fields: []value.FieldDescription{{MetadataID: "field-meta", Name: "Opaque", FullyQualifiedName: "[Opaque]"}}}},
	}, err: errors.New("later page failed")}
	got, err := resource.NewMetadataAdapter(identity, metadata, nil).ReadWorkbookSchema(t.Context(), "wb-1")
	if err == nil || len(got.Fields) != 1 || len(got.Fields[0]) != 1 || got.Fields[0][0].Role != "" || got.Fields[0][0].DataType != "" {
		t.Fatalf("got %+v, error %v", got, err)
	}
}

func TestWorkbookMetadataAdapterChecksRESTIdentityFirst(t *testing.T) {
	identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "other", Name: "Book"}}
	metadata := &workbookMetadataReader{}
	_, err := resource.NewMetadataAdapter(identity, metadata, nil).ReadWorkbookSchema(t.Context(), "wb-1")
	if err == nil || metadata.calls != 0 {
		t.Fatalf("metadata calls %d, error %v", metadata.calls, err)
	}
}

func TestWorkbookRemoteCalculationsKeepPublishedDependencySemantics(t *testing.T) {
	for _, tc := range []struct {
		name, formula, exclusion string
		userAggregation          bool
		hidden                   bool
	}{
		{"table calculation", "WINDOW_SUM(SUM([Sales]))", "table_calc", false, false},
		{"aggregate calculation", "SUM([Sales])", "", true, false},
		{"hidden table calculation", "WINDOW_SUM(SUM([Sales]))", "table_calc", false, true},
		{"hidden aggregate calculation", "SUM([Sales])", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "wb-1", Name: "Book"}}
			metadata := &workbookMetadataReader{result: value.MetadataWorkbookDatasources{
				Identity: value.MetadataIdentity{LUID: "wb-1", MetadataID: "wb-meta", Name: "Book"}, Complete: true,
				Datasources: []value.MetadataWorkbookDatasource{
					{Identity: value.MetadataIdentity{MetadataID: "local", Name: "Local"}, Kind: "embedded", Fields: []value.FieldDescription{
						{MetadataID: "local-base", Name: "Renamed Base", FullyQualifiedName: "[Calculation_1]", RemoteField: &value.MetadataIdentity{MetadataID: "base"}},
						{MetadataID: "local-dependent", Name: "Renamed Dependent", FullyQualifiedName: "[Calculation_2]", RemoteField: &value.MetadataIdentity{MetadataID: "dependent"}},
						{MetadataID: "local-calculation", Name: "Local Calculation", FullyQualifiedName: "[Calculation_3]", Type: "CalculatedField", Role: "MEASURE", DataType: "REAL", Formula: "[Renamed Dependent] + 1"},
					}},
					{Identity: value.MetadataIdentity{MetadataID: "published", LUID: "ds-1", Name: "Published"}, Kind: "published", Fields: []value.FieldDescription{
						{MetadataID: "base", Name: "Original Base", FullyQualifiedName: "[Calculation_1]", Type: "CalculatedField", Role: "MEASURE", DataType: "REAL", Formula: tc.formula},
						{MetadataID: "dependent", Name: "Original Dependent", FullyQualifiedName: "[Calculation_2]", Type: "CalculatedField", Role: "MEASURE", DataType: "REAL", Formula: "[Original Base] + 1"},
					}},
				},
			}}
			published := &publishedSchemaReader{result: fieldcatalog.Schema{Fields: []value.SchemaField{
				{ID: "Calculation_1", Name: "Calculation_1", Caption: "Original Base", Role: "measure", DataType: "REAL", Formula: tc.formula, Excluded: tc.exclusion != "", ExclusionReason: tc.exclusion, RequiresUserAggregation: tc.userAggregation},
				{ID: "Calculation_2", Name: "Calculation_2", Caption: "Original Dependent", Role: "measure", DataType: "REAL", Formula: "[Original Base] + 1", Excluded: tc.exclusion != "", ExclusionReason: tc.exclusion, RequiresUserAggregation: tc.userAggregation},
			}}}
			if tc.hidden {
				published.result.DependencyFields, published.result.Fields = published.result.Fields, nil
			}
			got, err := resource.NewMetadataAdapter(identity, metadata, published).ReadWorkbookSchema(t.Context(), "wb-1")
			if err != nil {
				t.Fatal(err)
			}
			if tc.hidden && len(got.Fields[1]) != 0 {
				t.Fatalf("hidden source became selectable in published group: %+v", got.Fields[1])
			}
			for _, field := range got.Fields[0] {
				if (field.Metadata.MetadataID == "local-dependent" || field.Metadata.MetadataID == "local-calculation") && (field.ExclusionReason != tc.exclusion || field.RequiresUserAggregation != tc.userAggregation) {
					t.Fatalf("renamed dependent lost published semantics: %+v", field)
				}
			}
		})
	}
}

func TestWorkbookPublishedSchemaFailurePreservesEmbeddedEvidence(t *testing.T) {
	identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "wb-1", Name: "Book"}}
	metadata := &workbookMetadataReader{result: value.MetadataWorkbookDatasources{
		Identity: value.MetadataIdentity{LUID: "wb-1", MetadataID: "wb-meta", Name: "Book"}, Complete: true,
		Datasources: []value.MetadataWorkbookDatasource{
			{Identity: value.MetadataIdentity{MetadataID: "local", Name: "Local"}, Kind: "embedded", Complete: true, Fields: []value.FieldDescription{{MetadataID: "local-field", Name: "Sales", FullyQualifiedName: "[Sales]", Role: "MEASURE", DataType: "REAL"}}},
			{Identity: value.MetadataIdentity{MetadataID: "published", LUID: "ds-1", Name: "Published"}, Kind: "published", Complete: true},
		},
	}}
	wantErr := errors.New("published schema unavailable")
	published := &publishedSchemaReader{err: wantErr}
	got, err := resource.NewMetadataAdapter(identity, metadata, published).ReadWorkbookSchema(t.Context(), "wb-1")
	if !errors.Is(err, wantErr) || got.Metadata.Complete || got.Metadata.Datasources[1].Complete || !got.Metadata.Datasources[0].Complete || len(got.Fields[0]) != 1 || len(got.Fields[1]) != 0 {
		t.Fatalf("partial evidence: %+v, error %v", got, err)
	}
	if !metadata.result.Datasources[1].Complete {
		t.Fatal("adapter changed the provider observation")
	}
}

func TestWorkbookDependencySemanticsOverrideVDSDisplayHidingWithoutGuessingDuplicates(t *testing.T) {
	for _, duplicate := range []string{"", "display", "dependency"} {
		t.Run("duplicate="+duplicate, func(t *testing.T) {
			identity := &metadataIdentityClient{result: tableauworkbook.Workbook{LUID: "wb-1", Name: "Book"}}
			metadata := &workbookMetadataReader{result: value.MetadataWorkbookDatasources{
				Identity: value.MetadataIdentity{LUID: "wb-1", MetadataID: "wb-meta", Name: "Book"}, Complete: true,
				Datasources: []value.MetadataWorkbookDatasource{
					{Identity: value.MetadataIdentity{MetadataID: "local", Name: "Local"}, Kind: "embedded", Complete: true, Fields: []value.FieldDescription{
						{MetadataID: "alias", Name: "Visible Alias", FullyQualifiedName: "[Alias]", RemoteField: &value.MetadataIdentity{MetadataID: "source"}},
						{MetadataID: "calc", Name: "Local Calc", FullyQualifiedName: "[Calculation_2]", Type: "CalculatedField", Role: "MEASURE", DataType: "REAL", Formula: "[Visible Alias] + 1"},
					}},
					{Identity: value.MetadataIdentity{MetadataID: "published", LUID: "ds-1", Name: "Published"}, Kind: "published", Complete: true, Fields: []value.FieldDescription{{MetadataID: "source", Name: "Hidden Source", FullyQualifiedName: "[Calculation_1]"}}},
				},
			}}
			published := &publishedSchemaReader{result: fieldcatalog.Schema{
				Fields:           []value.SchemaField{{ID: "Calculation_1", Name: "Calculation_1", Caption: "Hidden Source", Role: "excluded", DataType: "REAL", Excluded: true, ExclusionReason: "internal", DefaultAggregation: "AGG"}},
				DependencyFields: []value.SchemaField{{ID: "Calculation_1", Name: "Calculation_1", Caption: "Hidden Source", Role: "measure", DataType: "REAL", DefaultAggregation: "AGG", RequiresUserAggregation: true}},
			}}
			if duplicate == "display" {
				published.result.Fields = append(published.result.Fields, published.result.Fields[0])
			}
			if duplicate == "dependency" {
				published.result.DependencyFields = append(published.result.DependencyFields, published.result.DependencyFields[0])
			}
			got, err := resource.NewMetadataAdapter(identity, metadata, published).ReadWorkbookSchema(t.Context(), "wb-1")
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range got.Fields[0] {
				if field.Excluded || field.RequiresUserAggregation != (duplicate == "") {
					t.Fatalf("source semantic precedence or duplicate protection failed: %+v", field)
				}
			}
			if !got.Fields[1][0].Excluded || got.Fields[1][0].ExclusionReason != "internal" {
				t.Fatalf("published display contract changed: %+v", got.Fields[1])
			}
		})
	}
}
