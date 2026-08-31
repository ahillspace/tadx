package workbook_test

import (
	"context"
	"testing"

	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableaumetadata "github.com/ahillspace/tadx/internal/tableau/metadata"
)

type metadataClient struct {
	items []tableaumetadata.PublishedDatasource
	err   error
}

func (c metadataClient) DirectPublishedDatasources(context.Context, string) ([]tableaumetadata.PublishedDatasource, error) {
	return c.items, c.err
}

func TestReferenceAdapterReturnsStableAuthoritativeReferences(t *testing.T) {
	adapter := resourceworkbook.NewReferenceAdapter(metadataClient{items: []tableaumetadata.PublishedDatasource{
		{LUID: "ds-2", Name: "Inventory", SiteLUID: "site-1"},
		{LUID: "ds-1", Name: "Sales", SiteLUID: "site-1"},
	}})
	items, err := adapter.PublishedDatasources(context.Background(), "wb-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].LUID != "ds-1" || items[1].LUID != "ds-2" {
		t.Fatalf("items = %#v", items)
	}
}
