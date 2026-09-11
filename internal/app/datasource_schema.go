package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	datasourceschema "github.com/ahillspace/tadx/actions/datasource/schema"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
)

func (c *remoteContentCommands) GetDatasourceSchema(ctx context.Context, input datasourceschema.Input) (datasourceschema.Output, error) {
	var validationErr error
	input, validationErr = datasourceschema.NormalizeInput(input)
	if validationErr != nil {
		return datasourceschema.Output{}, validationErr
	}
	if input.Cursor != "" {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return datasourceschema.Output{}, err
		}
		input.Environment, input.Site = environment.Alias, environment.SiteContentURL
		if err := datasourceschema.ValidateContinuation(input); err != nil {
			return datasourceschema.Output{}, err
		}
	}
	if input.Cache {
		return c.getCacheDatasourceSchema(ctx, input)
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return datasourceschema.Output{}, capabilitySetupError("datasource.schema.setup", "datasource.schema", input.Environment, connection.environment.SiteContentURL, "Datasource schema setup failed.", "Verify the selected environment, PAT variables, and Tableau connectivity.", err)
	}
	input.Environment = connection.environment.Alias
	input.Site = connection.environment.SiteContentURL
	datasourceClient := c.runtime.clients(connection).datasources
	reader := &datasourceSchemaReader{adapter: resourcedatasource.NewSchemaAdapter(datasourceClient, fieldcatalog.NewClient(connection.transport, connection.session, connection.environment.URL)), now: c.runtime.now}
	if input.Descriptions || input.Tags {
		reader.metadata = c.runtime.clients(connection).metadataAssets
	}
	output, err := datasourceschema.New(reader, c.runtime.now).Execute(ctx, input)
	if err != nil {
		return datasourceschema.Output{}, err
	}
	if warning := c.storeLiveDatasourceSchema(ctx, connection.environment, reader.result); warning != "" {
		output.Warnings = append(output.Warnings, warning)
	}
	return output, nil
}

type datasourceSchemaReader struct {
	metadata *metadataassets.Client
	adapter  *resourcedatasource.SchemaAdapter
	now      func() time.Time
	result   datasourceschema.Schema
}

func (r *datasourceSchemaReader) ReadDatasourceSchema(ctx context.Context, luid string) (datasourceschema.Schema, error) {
	result, err := r.adapter.ReadDatasourceSchema(ctx, luid)
	if err != nil {
		return datasourceschema.Schema{}, err
	}
	tables := make([]datasourceschema.Table, len(result.Tables))
	copy(tables, result.Tables)
	fields := make([]datasourceschema.Field, len(result.Fields))
	copy(fields, result.Fields)
	observedAt := ""
	if r.now != nil {
		observedAt = r.now().UTC().Format(time.RFC3339Nano)
	}
	r.result = datasourceschema.Schema{DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, Tables: tables, Fields: fields, Warnings: append([]string(nil), result.Warnings...), ObservedAt: observedAt, RequestID: result.RequestID}
	if r.metadata != nil {
		metadata, err := r.metadata.DatasourceFieldDescriptions(ctx, luid)
		if err != nil {
			return datasourceschema.Schema{}, err
		}
		r.result.Fields = resourcedatasource.EnrichFields(fields, metadata.Fields)
		r.result.DescriptionsObserved, r.result.TagsObserved = true, true
		if !metadata.Complete {
			r.result.Warnings = append(r.result.Warnings, "Metadata coverage is incomplete; unmatched fields and absent observations do not prove metadata is missing.")
		}
	}
	return r.result, nil
}

type datasourceSchemaDocument struct {
	Version int                     `json:"version"`
	Schema  datasourceschema.Schema `json:"schema"`
}

type cachedDatasourceSchemaReader struct{ schema datasourceschema.Schema }

func (r cachedDatasourceSchemaReader) ReadDatasourceSchema(context.Context, string) (datasourceschema.Schema, error) {
	return r.schema, nil
}

func (c *remoteContentCommands) getCacheDatasourceSchema(ctx context.Context, input datasourceschema.Input) (datasourceschema.Output, error) {
	environment, site, err := c.resolveCacheTarget(input.Environment)
	if err != nil {
		return datasourceschema.Output{}, capabilitySetupError("datasource.schema.cache.setup", "datasource.schema", input.Environment, "", "Cache datasource schema setup failed.", "Verify the selected environment and cache configuration.", err)
	}
	input.Environment, input.Site = environment, site
	kind := "datasource_schema"
	if input.Descriptions || input.Tags {
		kind = "datasource_schema_metadata"
	}
	result, err := c.cacheStore(input.Environment).ReadResources(ctx, cache.ResourceQuery{Environment: environment, Site: site, Kind: kind, LUID: strings.TrimSpace(input.DatasourceLUID), Limit: 1})
	if err != nil {
		return datasourceschema.Output{}, cacheReadError("datasource.schema", environment, site, err)
	}
	var document datasourceSchemaDocument
	if len(result.Entries) != 1 || json.Unmarshal(result.Entries[0].Payload, &document) != nil || document.Version != 1 {
		return datasourceschema.Output{}, &errs.Error{ID: "cache.detail_not_indexed", Kind: errs.KindOperation, Operation: "datasource.schema", Environment: environment, Site: site, Summary: "The cache does not contain a usable datasource schema projection.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --cache to query Tableau and update the cache."}
	}
	output, err := datasourceschema.New(cachedDatasourceSchemaReader{schema: document.Schema}, c.runtime.now).Execute(ctx, input)
	if err != nil {
		return datasourceschema.Output{}, err
	}
	observed := result.NewestObserved
	if observed.IsZero() {
		observed = result.Entries[0].ObservedAt
	}
	source := readsource.Cached(observed, result.Coverage, result.GenerationID, result.GeneratedAt, result.Stale)
	output.Source = &source
	output.RequestID = ""
	return output, nil
}

func (c *remoteContentCommands) storeLiveDatasourceSchema(ctx context.Context, environment config.Environment, schema datasourceschema.Schema) string {
	if strings.TrimSpace(schema.DatasourceLUID) == "" || strings.TrimSpace(schema.DatasourceName) == "" {
		return "Cache write-through skipped because the datasource schema identity was incomplete."
	}
	payload, err := json.Marshal(datasourceSchemaDocument{Version: 1, Schema: schema})
	if err != nil {
		return "Cache write-through failed; the live datasource schema remains authoritative."
	}
	observedAt, err := time.Parse(time.RFC3339Nano, schema.ObservedAt)
	if err != nil {
		observedAt = c.runtime.now().UTC()
	}
	entry := cache.ResourceEntry{Environment: environment.Alias, Site: environment.SiteContentURL, Kind: "datasource_schema", LUID: schema.DatasourceLUID, Name: schema.DatasourceName, Payload: payload, Coverage: "detail", ObservedAt: observedAt}
	entries := []cache.ResourceEntry{entry}
	if schema.DescriptionsObserved || schema.TagsObserved {
		// Metadata is a separate observation, not replaced by a later VDS-only read.
		enriched := entry
		enriched.Kind = "datasource_schema_metadata"
		schema.DescriptionsObserved, schema.TagsObserved = false, false
		schema.Fields = append([]datasourceschema.Field(nil), schema.Fields...)
		for i := range schema.Fields {
			schema.Fields[i].Metadata = nil
			schema.Fields[i].MetadataMatch = ""
		}
		basePayload, encodeErr := json.Marshal(datasourceSchemaDocument{Version: 1, Schema: schema})
		if encodeErr != nil {
			return "Cache write-through failed; the live datasource schema remains authoritative."
		}
		entries[0].Payload = basePayload
		entries = append(entries, enriched)
	}
	if err := c.runtime.cacheStore(environment).UpsertResources(ctx, entries); err != nil {
		return "Cache write-through failed; the live datasource schema remains authoritative."
	}
	return ""
}
