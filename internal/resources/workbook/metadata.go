package workbook

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
	"github.com/ahillspace/tadx/internal/value"
)

type MetadataIdentityClient interface {
	Get(context.Context, string) (tableauworkbook.Workbook, error)
}

type WorkbookMetadataReader interface {
	WorkbookDatasources(context.Context, string) (value.MetadataWorkbookDatasources, error)
}

// MetadataAdapter binds one authoritative workbook to its Metadata API field groups.
type MetadataAdapter struct {
	workbooks MetadataIdentityClient
	metadata  WorkbookMetadataReader
}

func NewMetadataAdapter(workbooks MetadataIdentityClient, metadata WorkbookMetadataReader) *MetadataAdapter {
	return &MetadataAdapter{workbooks: workbooks, metadata: metadata}
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
	metadata, metadataErr := a.metadata.WorkbookDatasources(ctx, luid)
	out := value.WorkbookSchema{Metadata: metadata, Fields: make([][]value.SchemaField, len(metadata.Datasources))}
	if metadata.Identity.LUID != "" && metadata.Identity.LUID != luid {
		return out, errors.Join(metadataErr, fmt.Errorf("Metadata API returned mismatched workbook LUID %q", metadata.Identity.LUID))
	}
	if metadataErr == nil && (metadata.Identity.LUID != luid || metadata.Identity.MetadataID == "" || metadata.Identity.Name == "") {
		return out, errors.New("Metadata API workbook identity is incomplete")
	}
	sourceFields := make(map[string]*value.FieldDescription)
	for _, group := range metadata.Datasources {
		if group.Kind != "published" {
			continue
		}
		for i := range group.Fields {
			field := &group.Fields[i]
			if field.MetadataID == "" {
				continue
			}
			if _, exists := sourceFields[field.MetadataID]; exists {
				sourceFields[field.MetadataID] = nil
			} else {
				sourceFields[field.MetadataID] = field
			}
		}
	}
	for i, group := range metadata.Datasources {
		fields := make([]value.FieldDescription, len(group.Fields))
		copy(fields, group.Fields)
		if group.Kind == "embedded" {
			for j := range fields {
				remote := fields[j].RemoteField
				if remote == nil {
					continue
				}
				source := sourceFields[remote.MetadataID]
				if source == nil {
					continue
				}
				if fields[j].Type == "" {
					fields[j].Type = source.Type
				}
				if fields[j].Role == "" {
					fields[j].Role = source.Role
				}
				if fields[j].DataType == "" {
					fields[j].DataType = source.DataType
				}
				if fields[j].Aggregation == "" {
					fields[j].Aggregation = source.Aggregation
				}
				if fields[j].Formula == "" {
					fields[j].Formula = source.Formula
				}
			}
		}
		out.Fields[i] = fieldcatalog.NormalizeMetadataFields(fields)
	}
	return out, metadataErr
}
