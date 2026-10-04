package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/value"
)

func TestSchemaPublisherKeepsBaseAndMetadataObservationsSeparate(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	port := SchemaPublisherPort{Store: func() SchemaCacheWriter { return store }, Environment: "dev", Site: "sandbox", Now: func() time.Time { return now }}
	full := datasource.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", ObservedAt: now.Format(time.RFC3339Nano), DescriptionsObserved: true, TagsObserved: true, Fields: []datasource.Field{{ID: "f-1", Caption: "Sales", Metadata: &value.FieldDescription{MetadataID: "m-1"}, MetadataMatch: "exact"}}}
	if warning := port.PublishDatasourceSchema(t.Context(), full); warning != "" {
		t.Fatalf("write-through warning: %s", warning)
	}
	for _, test := range []struct {
		kind     string
		metadata bool
	}{{"datasource_schema", false}, {"datasource_schema_metadata", true}} {
		result, err := store.ReadResources(t.Context(), cache.ResourceQuery{Environment: "dev", Site: "sandbox", Kind: test.kind, LUID: "ds-1", Limit: 1})
		if err != nil || len(result.Entries) != 1 {
			t.Fatalf("%s result=%+v err=%v", test.kind, result, err)
		}
		var document schemaDocument
		if err := json.Unmarshal(result.Entries[0].Payload, &document); err != nil {
			t.Fatal(err)
		}
		if document.Version != 1 || document.Schema.Fields[0].Metadata != nil != test.metadata || document.Schema.Fields[0].MetadataMatch != "" != test.metadata {
			t.Fatalf("%s document=%+v", test.kind, document)
		}
		reader := &CachedSchemaPort{Store: store, Environment: "dev", Site: "sandbox", Metadata: test.metadata}
		cached, err := reader.ReadDatasourceSchema(t.Context(), "ds-1")
		if err != nil || cached.DatasourceLUID != "ds-1" || reader.Source() == nil {
			t.Fatalf("%s cached=%+v source=%+v err=%v", test.kind, cached, reader.Source(), err)
		}
	}
	// A later base-only read must not erase the separately observed metadata.
	full.DescriptionsObserved, full.TagsObserved = false, false
	full.Fields[0].Metadata, full.Fields[0].MetadataMatch = nil, ""
	if warning := port.PublishDatasourceSchema(t.Context(), full); warning != "" {
		t.Fatal(warning)
	}
	metadata := &CachedSchemaPort{Store: store, Environment: "dev", Site: "sandbox", Metadata: true}
	cached, err := metadata.ReadDatasourceSchema(t.Context(), "ds-1")
	if err != nil || cached.Fields[0].Metadata == nil || !cached.DescriptionsObserved || !cached.TagsObserved {
		t.Fatalf("metadata observation lost: %+v err=%v", cached, err)
	}
}

type failingSchemaCacheWriter struct{}

func (failingSchemaCacheWriter) UpsertResources(context.Context, []cache.ResourceEntry) error {
	return errors.New("disk unavailable")
}

func TestSchemaPublisherFailureIsBestEffortWarning(t *testing.T) {
	port := SchemaPublisherPort{Store: func() SchemaCacheWriter { return failingSchemaCacheWriter{} }, Environment: "dev", Site: "sandbox", Now: time.Now}
	if warning := port.PublishDatasourceSchema(t.Context(), datasource.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales"}); warning != "Cache write-through failed; the live datasource schema remains authoritative." {
		t.Fatalf("warning=%q", warning)
	}
	if warning := port.PublishDatasourceSchema(t.Context(), datasource.SchemaRecord{}); warning != "Cache write-through skipped because the datasource schema identity was incomplete." {
		t.Fatalf("incomplete identity warning=%q", warning)
	}
}

func TestCachedSchemaRejectsUnusableVersionAndMissingDetail(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{{Environment: "dev", Site: "sandbox", Kind: "datasource_schema", LUID: "ds-1", Name: "Sales", Coverage: "detail", ObservedAt: now, Payload: []byte(`{"version":2,"schema":{}}`)}}); err != nil {
		t.Fatal(err)
	}
	reader := &CachedSchemaPort{Store: store, Environment: "dev", Site: "sandbox"}
	_, err := reader.ReadDatasourceSchema(t.Context(), "ds-1")
	var diagnostic SchemaDetailUnavailable
	if !errors.As(err, &diagnostic) || !diagnostic.SchemaDetailNotIndexed() {
		t.Fatalf("version error=%v", err)
	}
	sentinel := errors.New("mapped cache read error")
	reader.ReadError = func(operation, environment, site string, cause error) error {
		if operation != "datasource.schema" || environment != "dev" || site != "sandbox" || cause == nil {
			t.Fatalf("cache error context=%s/%s/%s: %v", operation, environment, site, cause)
		}
		return sentinel
	}
	_, err = reader.ReadDatasourceSchema(t.Context(), "missing")
	if !errors.Is(err, sentinel) {
		t.Fatalf("mapped cache read error=%v", err)
	}
}
