package workbook

import (
	"context"
	"errors"
	"sort"
	"strings"

	tableaumetadata "github.com/ahillspace/tadx/internal/tableau/metadata"
)

// ReferenceClient is the focused Metadata API dependency reader.
type ReferenceClient interface {
	DirectPublishedDatasources(context.Context, string) ([]tableaumetadata.PublishedDatasource, error)
}

// PublishedDatasource is one direct workbook dependency.
type PublishedDatasource struct {
	LUID string
	Name string
}

// ReferenceAdapter isolates workbook lineage metadata from actions.
type ReferenceAdapter struct{ client ReferenceClient }

// NewReferenceAdapter creates a workbook reference adapter.
func NewReferenceAdapter(client ReferenceClient) *ReferenceAdapter {
	return &ReferenceAdapter{client: client}
}

// PublishedDatasources returns stable direct references by authoritative REST LUID.
func (a *ReferenceAdapter) PublishedDatasources(ctx context.Context, workbookLUID string) ([]PublishedDatasource, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("workbook reference adapter is not configured")
	}
	items, err := a.client.DirectPublishedDatasources(ctx, workbookLUID)
	if err != nil {
		return nil, err
	}
	result := make([]PublishedDatasource, len(items))
	for index, item := range items {
		if strings.TrimSpace(item.LUID) == "" {
			return nil, errors.New("published datasource reference omitted its authoritative LUID")
		}
		result[index] = PublishedDatasource{LUID: strings.TrimSpace(item.LUID), Name: strings.TrimSpace(item.Name)}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].LUID < result[right].LUID })
	return result, nil
}
