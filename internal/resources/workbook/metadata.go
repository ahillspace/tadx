package workbook

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
	"github.com/ahillspace/tadx/internal/value"
)

type MetadataIdentityClient interface {
	Get(context.Context, string) (tableauworkbook.Workbook, error)
}

type WorkbookMetadataReader interface {
	WorkbookFieldDescriptions(context.Context, string) (value.MetadataWorkbookDatasources, error)
}

// PublishedSchemaReader reuses the authoritative datasource schema owner.
type PublishedSchemaReader interface {
	ReadDatasourceSchema(context.Context, string) (fieldcatalog.Schema, error)
}

// MetadataAdapter binds one authoritative workbook to its Metadata API field groups.
type MetadataAdapter struct {
	workbooks MetadataIdentityClient
	metadata  WorkbookMetadataReader
	published PublishedSchemaReader
}

func NewMetadataAdapter(workbooks MetadataIdentityClient, metadata WorkbookMetadataReader, published PublishedSchemaReader) *MetadataAdapter {
	return &MetadataAdapter{workbooks: workbooks, metadata: metadata, published: published}
}

func (a *MetadataAdapter) ReadWorkbookSchema(ctx context.Context, luid string) (value.WorkbookSchema, error) {
	if a == nil || a.workbooks == nil || a.metadata == nil {
		return value.WorkbookSchema{}, errors.New("workbook metadata adapter is not configured")
	}
	if luid == "" || luid != strings.TrimSpace(luid) {
		return value.WorkbookSchema{}, errors.New("an exact workbook LUID is required")
	}
	wb, err := a.workbooks.Get(ctx, luid)
	if err != nil {
		return value.WorkbookSchema{}, err
	}
	if wb.LUID != luid || strings.TrimSpace(wb.Name) == "" {
		return value.WorkbookSchema{}, fmt.Errorf("workbook identity response was incomplete or mismatched for LUID %q", luid)
	}
	metadata, metadataErr := a.metadata.WorkbookFieldDescriptions(ctx, luid)
	out := value.WorkbookSchema{Metadata: metadata, Fields: make([][]value.SchemaField, len(metadata.Datasources))}
	out.Metadata.Datasources = slices.Clone(metadata.Datasources)
	out.Metadata.Warnings = slices.Clone(metadata.Warnings)
	if metadata.Identity.LUID != "" && metadata.Identity.LUID != luid {
		return out, errors.Join(metadataErr, fmt.Errorf("Metadata API returned mismatched workbook LUID %q", metadata.Identity.LUID))
	}
	if metadataErr == nil && (metadata.Identity.LUID != luid || metadata.Identity.MetadataID == "" || metadata.Identity.Name == "") {
		return out, errors.New("Metadata API workbook identity is incomplete")
	}
	sourceFields := make(map[string]value.SchemaField)
	duplicateSourceIDs := make(map[string]bool)
	var schemaErrors []error
	for i, group := range metadata.Datasources {
		if group.Kind != "published" {
			continue
		}
		if a.published == nil || group.Identity.LUID == "" {
			out.Metadata.Complete, out.Metadata.Datasources[i].Complete = false, false
			schemaErrors = append(schemaErrors, fmt.Errorf("published datasource %q lacks a configured schema reader or exact LUID", group.Identity.MetadataID))
			continue
		}
		schema, err := a.published.ReadDatasourceSchema(ctx, group.Identity.LUID)
		if err != nil {
			out.Metadata.Complete, out.Metadata.Datasources[i].Complete = false, false
			schemaErrors = append(schemaErrors, err)
			continue
		}
		out.Fields[i] = value.EnrichSchemaFields(schema.Fields, group.Fields)
		out.Metadata.Warnings = append(out.Metadata.Warnings, schema.Warnings...)
		dependencies := value.EnrichSchemaFields(schema.DependencyFields, group.Fields)
		groupSources, duplicates := schemaDependencySources(out.Fields[i], dependencies)
		for id := range duplicates {
			duplicateSourceIDs[id] = true
			delete(sourceFields, id)
		}
		for id, field := range groupSources {
			if duplicateSourceIDs[id] {
				continue
			}
			if _, exists := sourceFields[id]; exists {
				delete(sourceFields, id)
				duplicateSourceIDs[id] = true
			} else {
				sourceFields[id] = field
			}
		}
	}
	for i, group := range metadata.Datasources {
		if group.Kind == "embedded" {
			out.Fields[i] = fieldcatalog.NormalizeMetadataFields(group.Fields, sourceFields)
		}
	}
	return out, errors.Join(append(schemaErrors, metadataErr)...)
}

// schemaDependencySources prefers semantic facts over the same field's display
// placeholder while rejecting ambiguous joins within either source inventory.
func schemaDependencySources(fields, dependencies []value.SchemaField) (map[string]value.SchemaField, map[string]bool) {
	sources := make(map[string]value.SchemaField)
	duplicates := make(map[string]bool)
	for _, inventory := range [][]value.SchemaField{fields, dependencies} {
		seen := make(map[string]bool)
		for _, field := range inventory {
			if field.Metadata == nil || field.Metadata.MetadataID == "" || field.MetadataMatch != "exact" {
				continue
			}
			id := field.Metadata.MetadataID
			prior, exists := sources[id]
			if seen[id] || exists && (prior.ID != field.ID || prior.Name != field.Name) {
				duplicates[id] = true
				delete(sources, id)
			}
			seen[id] = true
			if !duplicates[id] {
				sources[id] = field
			}
		}
	}
	return sources, duplicates
}
