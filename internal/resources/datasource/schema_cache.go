package datasource

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/readsource"
)

type SchemaDetailUnavailable struct{}

func (SchemaDetailUnavailable) Error() string {
	return "cached datasource schema detail is unavailable"
}
func (SchemaDetailUnavailable) SchemaDetailNotIndexed() bool { return true }

type schemaDocument struct {
	Version int                     `json:"version"`
	Schema  datasource.SchemaRecord `json:"schema"`
}

type SchemaCacheReader interface {
	ReadResources(context.Context, cache.ResourceQuery) (cache.ResourceResult, error)
}

type SchemaCacheWriter interface {
	UpsertResources(context.Context, []cache.ResourceEntry) error
}

// CachedSchemaPort reads one versioned schema projection and its provenance.
type CachedSchemaPort struct {
	Store       SchemaCacheReader
	Environment string
	Site        string
	Metadata    bool
	ReadError   func(string, string, string, error) error
	source      *readsource.Metadata
}

func (p *CachedSchemaPort) ReadDatasourceSchema(ctx context.Context, luid string) (datasource.SchemaRecord, error) {
	kind := "datasource_schema"
	if p.Metadata {
		kind = "datasource_schema_metadata"
	}
	result, err := p.Store.ReadResources(ctx, cache.ResourceQuery{Environment: p.Environment, Site: p.Site, Kind: kind, LUID: strings.TrimSpace(luid), Limit: 1})
	if err != nil {
		if p.ReadError != nil {
			return datasource.SchemaRecord{}, p.ReadError("datasource.schema", p.Environment, p.Site, err)
		}
		return datasource.SchemaRecord{}, err
	}
	var document schemaDocument
	if len(result.Entries) != 1 || json.Unmarshal(result.Entries[0].Payload, &document) != nil || document.Version != 1 {
		return datasource.SchemaRecord{}, SchemaDetailUnavailable{}
	}
	observed := result.NewestObserved
	if observed.IsZero() {
		observed = result.Entries[0].ObservedAt
	}
	source := readsource.Cached(observed, result.Coverage, result.GenerationID, result.GeneratedAt, result.Stale)
	p.source = &source
	return document.Schema, nil
}

func (p *CachedSchemaPort) Source() *readsource.Metadata { return p.source }

// SchemaPublisherPort persists live base and metadata observations independently.
type SchemaPublisherPort struct {
	Store       func() SchemaCacheWriter
	Environment string
	Site        string
	Now         func() time.Time
}

func (p SchemaPublisherPort) PublishDatasourceSchema(ctx context.Context, schema datasource.SchemaRecord) string {
	if strings.TrimSpace(schema.DatasourceLUID) == "" || strings.TrimSpace(schema.DatasourceName) == "" {
		return "Cache write-through skipped because the datasource schema identity was incomplete."
	}
	payload, err := json.Marshal(schemaDocument{Version: 1, Schema: schema})
	if err != nil {
		return "Cache write-through failed; the live datasource schema remains authoritative."
	}
	observedAt, err := time.Parse(time.RFC3339Nano, schema.ObservedAt)
	if err != nil {
		observedAt = p.Now().UTC()
	}
	entry := cache.ResourceEntry{Environment: p.Environment, Site: p.Site, Kind: "datasource_schema", LUID: schema.DatasourceLUID, Name: schema.DatasourceName, Payload: payload, Coverage: "detail", ObservedAt: observedAt}
	entries := []cache.ResourceEntry{entry}
	if schema.DescriptionsObserved || schema.TagsObserved {
		// A later VDS-only read must not replace the separate metadata observation.
		enriched := entry
		enriched.Kind = "datasource_schema_metadata"
		schema.DescriptionsObserved, schema.TagsObserved = false, false
		schema.Fields = append([]datasource.Field(nil), schema.Fields...)
		for i := range schema.Fields {
			schema.Fields[i].Metadata = nil
			schema.Fields[i].MetadataMatch = ""
		}
		basePayload, encodeErr := json.Marshal(schemaDocument{Version: 1, Schema: schema})
		if encodeErr != nil {
			return "Cache write-through failed; the live datasource schema remains authoritative."
		}
		entries[0].Payload = basePayload
		entries = append(entries, enriched)
	}
	if err := p.Store().UpsertResources(ctx, entries); err != nil {
		return "Cache write-through failed; the live datasource schema remains authoritative."
	}
	return ""
}
