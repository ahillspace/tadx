// Package catalog adapts supported upstream metadata operations without changing ID namespaces.
package catalog

import (
	"context"

	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

type Adapter struct{ *metadataassets.Client }

func New(client *metadataassets.Client) *Adapter { return &Adapter{Client: client} }

func (a *Adapter) AddDatabaseTags(ctx context.Context, id string, tags []string) ([]string, error) {
	return a.Client.AddTags(ctx, metadataassets.LabelTarget{Type: "database", LUID: id}, tags)
}
func (a *Adapter) DeleteDatabaseTag(ctx context.Context, id, tag string) error {
	return a.Client.DeleteTag(ctx, metadataassets.LabelTarget{Type: "database", LUID: id}, tag)
}

func (a *Adapter) AddTableTags(ctx context.Context, id string, tags []string) ([]string, error) {
	return a.Client.AddTags(ctx, metadataassets.LabelTarget{Type: "table", LUID: id}, tags)
}
func (a *Adapter) DeleteTableTag(ctx context.Context, id, tag string) error {
	return a.Client.DeleteTag(ctx, metadataassets.LabelTarget{Type: "table", LUID: id}, tag)
}

func (a *Adapter) AddColumnTags(ctx context.Context, id string, tags []string) ([]string, error) {
	return a.Client.AddTags(ctx, metadataassets.LabelTarget{Type: "column", LUID: id}, tags)
}
func (a *Adapter) DeleteColumnTag(ctx context.Context, id, tag string) error {
	return a.Client.DeleteTag(ctx, metadataassets.LabelTarget{Type: "column", LUID: id}, tag)
}
