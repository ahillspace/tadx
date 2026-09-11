// Package catalog adapts supported upstream metadata operations without changing ID namespaces.
package catalog

import (
	"context"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
	"github.com/ahillspace/tadx/internal/value"
)

type Adapter struct{ client *metadataassets.Client }

func New(client *metadataassets.Client) *Adapter { return &Adapter{client: client} }

func (a *Adapter) DiscoverDatabases(ctx context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	return a.client.DiscoverDatabases(ctx, q)
}
func (a *Adapter) GetDatabase(ctx context.Context, id string) (value.MetadataDatabase, error) {
	return a.client.GetDatabase(ctx, id)
}
func (a *Adapter) UpdateDatabase(ctx context.Context, id string, patch value.MetadataUpdate) (value.MetadataDatabase, error) {
	return a.client.UpdateDatabase(ctx, id, patch)
}
func (a *Adapter) AddDatabaseTags(ctx context.Context, id string, tags []string) ([]string, error) {
	return a.client.AddTags(ctx, metadataassets.LabelTarget{Type: "database", LUID: id}, tags)
}
func (a *Adapter) DeleteDatabaseTag(ctx context.Context, id, tag string) error {
	return a.client.DeleteTag(ctx, metadataassets.LabelTarget{Type: "database", LUID: id}, tag)
}

func (a *Adapter) DiscoverTables(ctx context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	return a.client.DiscoverTables(ctx, q)
}
func (a *Adapter) GetTable(ctx context.Context, id string) (value.MetadataTable, error) {
	return a.client.GetTable(ctx, id)
}
func (a *Adapter) UpdateTable(ctx context.Context, id string, patch value.MetadataUpdate) (value.MetadataTable, error) {
	return a.client.UpdateTable(ctx, id, patch)
}
func (a *Adapter) AddTableTags(ctx context.Context, id string, tags []string) ([]string, error) {
	return a.client.AddTags(ctx, metadataassets.LabelTarget{Type: "table", LUID: id}, tags)
}
func (a *Adapter) DeleteTableTag(ctx context.Context, id, tag string) error {
	return a.client.DeleteTag(ctx, metadataassets.LabelTarget{Type: "table", LUID: id}, tag)
}

func (a *Adapter) DiscoverColumns(ctx context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	return a.client.DiscoverColumns(ctx, q)
}
func (a *Adapter) GetColumn(ctx context.Context, parent, id string) (value.MetadataColumn, error) {
	return a.client.GetColumn(ctx, parent, id)
}
func (a *Adapter) UpdateColumn(ctx context.Context, parent, id string, patch value.MetadataUpdate) (value.MetadataColumn, error) {
	return a.client.UpdateColumn(ctx, parent, id, patch)
}
func (a *Adapter) AddColumnTags(ctx context.Context, id string, tags []string) ([]string, error) {
	return a.client.AddTags(ctx, metadataassets.LabelTarget{Type: "column", LUID: id}, tags)
}
func (a *Adapter) DeleteColumnTag(ctx context.Context, id, tag string) error {
	return a.client.DeleteTag(ctx, metadataassets.LabelTarget{Type: "column", LUID: id}, tag)
}
func (a *Adapter) DatasourceFieldDescriptions(ctx context.Context, id string) (value.MetadataDatasourceDescriptions, error) {
	return a.client.DatasourceFieldDescriptions(ctx, id)
}
