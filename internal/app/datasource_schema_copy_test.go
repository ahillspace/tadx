package app

import (
	"context"
	"testing"

	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
)

type sharedSchemaFixture struct{ result fieldcatalog.Schema }

func (f *sharedSchemaFixture) Get(context.Context, string) (tableaudatasource.Datasource, error) {
	return tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales"}, nil
}

func (f *sharedSchemaFixture) Read(context.Context, string, string) (fieldcatalog.Schema, error) {
	return f.result, nil
}

func TestSharedSchemaValuesRetainSliceIsolation(t *testing.T) {
	fixture := &sharedSchemaFixture{result: fieldcatalog.Schema{
		DatasourceLUID: "ds-1", DatasourceName: "Sales",
		Tables:   []fieldcatalog.Table{{ID: "orders", Name: "Orders", FieldCount: 1}},
		Fields:   []fieldcatalog.Field{{ID: "sales", Caption: "Sales", Role: "measure"}},
		Warnings: []string{"Original warning"},
	}}
	reader := &datasourceSchemaReader{adapter: resourcedatasource.NewSchemaAdapter(fixture, fixture)}
	result, err := reader.ReadDatasourceSchema(context.Background(), "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	fixture.result.Fields[0].Caption = "Changed provider field"
	fixture.result.Tables[0].Name = "Changed provider table"
	fixture.result.Warnings[0] = "Changed provider warning"
	if result.Fields[0].Caption != "Sales" || result.Tables[0].Name != "Orders" || result.Warnings[0] != "Original warning" {
		t.Fatalf("composition root retained provider slice aliases: %+v", result)
	}
	fixture.result.Fields, fixture.result.Tables = nil, nil
	empty, err := reader.ReadDatasourceSchema(context.Background(), "ds-1")
	if err != nil || empty.Fields == nil || empty.Tables == nil {
		t.Fatalf("allocated empty schema slices changed: %+v err=%v", empty, err)
	}
}
