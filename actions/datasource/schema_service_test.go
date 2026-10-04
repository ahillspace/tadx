package datasource_test

import (
	"context"
	"errors"
	"testing"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

type schemaCanonicalProvider struct {
	reader     *schemaReader
	target     datasource.SchemaTarget
	openCalls  int
	cacheCalls int
}

func (p *schemaCanonicalProvider) CursorTarget(string) (datasource.SchemaTarget, error) {
	return p.target, nil
}
func (p *schemaCanonicalProvider) CacheTarget(string) (datasource.SchemaTarget, error) {
	return p.target, nil
}
func (p *schemaCanonicalProvider) CachedSchema(datasource.SchemaTarget, bool) datasource.CachedSchemaReader {
	p.cacheCalls++
	source := readsource.Cached(time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC), readsource.CoveragePartial, "generation-1", time.Time{}, true)
	return schemaTestCachedReader{SchemaReader: p.reader, source: &source}
}
func (p *schemaCanonicalProvider) OpenDatasourceSchema(context.Context, string, bool) (datasource.SchemaSession, error) {
	p.openCalls++
	return datasource.SchemaSession{SchemaTarget: p.target, Reader: p.reader, Publisher: schemaTestPublisher{}}, nil
}
func (*schemaCanonicalProvider) Now() time.Time { return time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC) }

func TestSchemaServiceBindsContinuationToCanonicalTarget(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "cache"}[cached], func(t *testing.T) {
			provider := &schemaCanonicalProvider{reader: &schemaReader{result: datasource.SchemaRecord{DatasourceLUID: "ds-1", DatasourceName: "Sales", Fields: []datasource.Field{{ID: "a", Caption: "A"}, {ID: "b", Caption: "B"}}}}, target: datasource.SchemaTarget{Environment: "production", Site: "canonical-site"}}
			service := datasource.New(datasource.Ports{Schema: provider})
			input := datasource.SchemaInput{Environment: "alias", DatasourceLUID: "ds-1", Limit: 1, Cache: cached}
			first, err := service.GetDatasourceSchema(t.Context(), input)
			if err != nil || first.Page.NextCursor == "" || first.Environment != "production" || first.Site != "canonical-site" {
				t.Fatalf("first=%+v err=%v", first, err)
			}
			input.Cursor = first.Page.NextCursor
			second, err := service.GetDatasourceSchema(t.Context(), input)
			if err != nil || second.Page.Returned != 1 || second.Fields[0].ID != "b" {
				t.Fatalf("second=%+v err=%v", second, err)
			}
			if cached && (provider.openCalls != 0 || provider.cacheCalls != 2 || second.RequestID != "") {
				t.Fatalf("cache opened provider or lost provenance: %+v output=%+v", provider, second)
			}
			if !cached && (provider.openCalls != 2 || provider.cacheCalls != 0) {
				t.Fatalf("live read calls=%+v", provider)
			}
			provider.target.Site = "other-site"
			before := provider.openCalls
			_, err = service.GetDatasourceSchema(t.Context(), input)
			var diagnostic *errs.Error
			if !errors.As(err, &diagnostic) || diagnostic.ID != "datasource.schema.cursor" || provider.openCalls != before {
				t.Fatalf("changed canonical target err=%v open=%d/%d", err, provider.openCalls, before)
			}
		})
	}
}

type unavailableSchemaDetail struct{}

func (unavailableSchemaDetail) Error() string                { return "detail unavailable" }
func (unavailableSchemaDetail) SchemaDetailNotIndexed() bool { return true }

func TestSchemaServiceClassifiesUnusableCacheDetail(t *testing.T) {
	provider := &schemaCanonicalProvider{reader: &schemaReader{err: unavailableSchemaDetail{}}, target: datasource.SchemaTarget{Environment: "dev", Site: "sandbox"}}
	_, err := datasource.New(datasource.Ports{Schema: provider}).GetDatasourceSchema(t.Context(), datasource.SchemaInput{Environment: "dev", DatasourceLUID: "ds-1", Cache: true})
	var diagnostic *errs.Error
	if !errors.As(err, &diagnostic) || diagnostic.ID != "cache.detail_not_indexed" || diagnostic.Environment != "dev" || diagnostic.Site != "sandbox" || provider.openCalls != 0 {
		t.Fatalf("cache diagnostic=%v provider=%+v", err, provider)
	}
}
