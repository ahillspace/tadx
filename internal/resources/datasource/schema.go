package datasource

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
)

// SchemaClient retrieves normalized datasource field metadata.
type SchemaClient interface {
	Read(context.Context, string, string) (fieldcatalog.Schema, error)
}

// SchemaIdentityClient retrieves the authoritative datasource identity.
type SchemaIdentityClient interface {
	Get(context.Context, string) (tableaudatasource.Datasource, error)
}

// SchemaAdapter owns exact datasource schema sequencing.
type SchemaAdapter struct {
	datasources SchemaIdentityClient
	fields      SchemaClient
}

// NewSchemaAdapter creates a datasource schema adapter.
func NewSchemaAdapter(datasources SchemaIdentityClient, fields SchemaClient) *SchemaAdapter {
	return &SchemaAdapter{datasources: datasources, fields: fields}
}

// ReadDatasourceSchema validates one authoritative datasource and returns its complete schema.
func (a *SchemaAdapter) ReadDatasourceSchema(ctx context.Context, datasourceLUID string) (fieldcatalog.Schema, error) {
	if a == nil || a.datasources == nil || a.fields == nil {
		return fieldcatalog.Schema{}, errors.New("datasource schema adapter is not configured")
	}
	datasourceLUID = strings.TrimSpace(datasourceLUID)
	if datasourceLUID == "" {
		return fieldcatalog.Schema{}, errors.New("datasource LUID is required")
	}
	datasource, err := a.datasources.Get(ctx, datasourceLUID)
	if err != nil {
		return fieldcatalog.Schema{}, err
	}
	if strings.TrimSpace(datasource.LUID) != datasourceLUID || strings.TrimSpace(datasource.Name) == "" {
		return fieldcatalog.Schema{}, fmt.Errorf("datasource identity response was incomplete or mismatched for LUID %q", datasourceLUID)
	}
	result, err := a.fields.Read(ctx, datasourceLUID, datasource.Name)
	if err != nil {
		return fieldcatalog.Schema{}, err
	}
	if result.DatasourceLUID != datasourceLUID || strings.TrimSpace(result.DatasourceName) == "" {
		return fieldcatalog.Schema{}, fmt.Errorf("datasource schema response was incomplete or mismatched for LUID %q", datasourceLUID)
	}
	for index, field := range result.Fields {
		if strings.TrimSpace(field.ID) == "" || strings.TrimSpace(field.Caption) == "" || !validSchemaRole(field.Role) {
			return fieldcatalog.Schema{}, fmt.Errorf("datasource schema field %d is incomplete", index)
		}
	}
	return result, nil
}

func validSchemaRole(value string) bool {
	switch value {
	case "measure", "dimension", "date", "excluded":
		return true
	default:
		return false
	}
}
