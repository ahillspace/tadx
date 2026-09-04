package datasource_test

import (
	"context"
	"errors"
	"testing"

	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
)

type schemaDatasourceClient struct {
	result tableaudatasource.Datasource
	err    error
}

func (c schemaDatasourceClient) Get(context.Context, string) (tableaudatasource.Datasource, error) {
	return c.result, c.err
}

type schemaFieldClient struct {
	result fieldcatalog.Schema
	err    error
	name   string
}

func (c *schemaFieldClient) Read(_ context.Context, _ string, name string) (fieldcatalog.Schema, error) {
	c.name = name
	return c.result, c.err
}

func TestSchemaAdapterUsesAuthoritativeDatasourceName(t *testing.T) {
	fields := &schemaFieldClient{result: fieldcatalog.Schema{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []fieldcatalog.Field{{ID: "Sales", Caption: "Sales", Role: "measure"}}}}
	adapter := resourcedatasource.NewSchemaAdapter(schemaDatasourceClient{result: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales"}}, fields)
	got, err := adapter.ReadDatasourceSchema(context.Background(), "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	if fields.name != "Sales" || len(got.Fields) != 1 {
		t.Fatalf("name=%q schema=%#v", fields.name, got)
	}
}

func TestSchemaAdapterStopsBeforeFieldsOnIdentityFailure(t *testing.T) {
	fields := &schemaFieldClient{}
	adapter := resourcedatasource.NewSchemaAdapter(schemaDatasourceClient{err: errors.New("not found")}, fields)
	if _, err := adapter.ReadDatasourceSchema(context.Background(), "ds-1"); err == nil {
		t.Fatal("expected identity error")
	}
	if fields.name != "" {
		t.Fatalf("field client called with %q", fields.name)
	}
}
