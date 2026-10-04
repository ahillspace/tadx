package datasource

import (
	"context"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

// SchemaMetadata reads the optional metadata observation for one datasource.
type SchemaMetadata interface {
	DatasourceFieldDescriptions(context.Context, string) (metadataassets.DatasourceDescriptions, error)
}

// SchemaReadPort owns the Tableau schema projection and optional enrichment.
type SchemaReadPort struct {
	Adapter  *SchemaAdapter
	Metadata SchemaMetadata
	Now      func() time.Time
}

func (p SchemaReadPort) ReadDatasourceSchema(ctx context.Context, luid string) (datasource.SchemaRecord, error) {
	result, err := p.Adapter.ReadDatasourceSchema(ctx, luid)
	if err != nil {
		return datasource.SchemaRecord{}, err
	}
	tables := make([]datasource.Table, len(result.Tables))
	copy(tables, result.Tables)
	fields := make([]datasource.Field, len(result.Fields))
	copy(fields, result.Fields)
	observedAt := ""
	if p.Now != nil {
		observedAt = p.Now().UTC().Format(time.RFC3339Nano)
	}
	record := datasource.SchemaRecord{DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, Tables: tables, Fields: fields, Warnings: append([]string(nil), result.Warnings...), ObservedAt: observedAt, RequestID: result.RequestID}
	if p.Metadata != nil {
		metadata, err := p.Metadata.DatasourceFieldDescriptions(ctx, luid)
		if err != nil {
			return datasource.SchemaRecord{}, err
		}
		record.Fields = EnrichFields(fields, metadata.Fields)
		record.DescriptionsObserved, record.TagsObserved = true, true
		if !metadata.Complete {
			record.Warnings = append(record.Warnings, "Metadata coverage is incomplete; unmatched fields and absent observations do not prove metadata is missing.")
		}
	}
	return record, nil
}
