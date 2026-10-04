package datasource

import (
	"context"
	"errors"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

type SchemaTarget struct{ Environment, Site string }

type CachedSchemaReader interface {
	SchemaReader
	Source() *readsource.Metadata
}

type SchemaPublisher interface {
	PublishDatasourceSchema(context.Context, SchemaRecord) string
}

type SchemaSession struct {
	SchemaTarget
	Reader    SchemaReader
	Publisher SchemaPublisher
}

type SchemaProvider interface {
	CursorTarget(string) (SchemaTarget, error)
	CacheTarget(string) (SchemaTarget, error)
	CachedSchema(SchemaTarget, bool) CachedSchemaReader
	OpenDatasourceSchema(context.Context, string, bool) (SchemaSession, error)
	Now() time.Time
}

func (s *Service) GetDatasourceSchema(ctx context.Context, input SchemaInput) (SchemaOutput, error) {
	var err error
	input, err = schemaNormalizeInput(input)
	if err != nil {
		return SchemaOutput{}, err
	}
	if s == nil || s.ports.Schema == nil {
		return SchemaOutput{}, schemaSchemaError("datasource.schema.unconfigured", errs.KindRuntime, input, "Datasource schema discovery is not configured.", nil, "Configure the datasource schema reader before retrying.")
	}
	p := s.ports.Schema
	var target SchemaTarget
	var offset int
	if input.Cursor != "" {
		target, err = p.CursorTarget(input.Environment)
		if err != nil {
			return SchemaOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		offset, err = schemaContinuationOffset(input)
		if err != nil {
			return SchemaOutput{}, err
		}
	}
	if input.Cache {
		if input.Cursor == "" {
			target, err = p.CacheTarget(input.Environment)
			if err != nil {
				return SchemaOutput{}, err
			}
			input.Environment, input.Site = target.Environment, target.Site
		}
		reader := p.CachedSchema(target, input.Descriptions || input.Tags)
		record, err := reader.ReadDatasourceSchema(ctx, input.DatasourceLUID)
		if err != nil {
			var unavailable interface{ SchemaDetailNotIndexed() bool }
			if errors.As(err, &unavailable) && unavailable.SchemaDetailNotIndexed() {
				return SchemaOutput{}, &errs.Error{ID: "cache.detail_not_indexed", Kind: errs.KindOperation, Operation: "datasource.schema", Environment: input.Environment, Site: input.Site, Summary: "The cache does not contain a usable datasource schema projection.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --cache to query Tableau and update the cache."}
			}
			return SchemaOutput{}, err
		}
		output, err := schemaValidated(ctx, schemaRecordReader{record}, p.Now, input, offset, nil)
		if err != nil {
			return SchemaOutput{}, err
		}
		output.Source = reader.Source()
		output.RequestID = ""
		return output, nil
	}
	session, err := p.OpenDatasourceSchema(ctx, input.Environment, input.Descriptions || input.Tags)
	if err != nil {
		return SchemaOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if input.Cursor != "" && target != session.SchemaTarget {
		offset, err = schemaContinuationOffset(input)
		if err != nil {
			return SchemaOutput{}, err
		}
	}
	var complete SchemaRecord
	output, err := schemaValidated(ctx, session.Reader, p.Now, input, offset, &complete)
	if err != nil {
		return SchemaOutput{}, err
	}
	if warning := session.Publisher.PublishDatasourceSchema(ctx, complete); warning != "" {
		output.Warnings = append(output.Warnings, warning)
	}
	return output, nil
}

type schemaRecordReader struct{ record SchemaRecord }

func (r schemaRecordReader) ReadDatasourceSchema(context.Context, string) (SchemaRecord, error) {
	return r.record, nil
}
