package pulse

import (
	"context"
	"errors"
	"fmt"
	"strings"

	definition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type DefinitionMutationPort struct{ Client *tableaupulse.Client }

func (a *DefinitionMutationPort) FindDefinitions(ctx context.Context, name, datasourceLUID string) ([]definition.Definition, error) {
	result := []definition.Definition{}
	token := ""
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := a.Client.ListDefinitions(ctx, tableaupulse.PageRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return nil, err
		}
		for _, item := range page.Definitions {
			if item.Name == name && item.DatasourceLUID == datasourceLUID {
				result = append(result, definition.Definition{LUID: item.LUID, Name: item.Name, DatasourceLUID: item.DatasourceLUID})
			}
		}
		if page.NextPageToken == "" {
			return result, nil
		}
		token = page.NextPageToken
	}
	return nil, errors.New("Pulse definition collision scan exceeded 100 pages")
}

func (a DefinitionMutationPort) GetDefinition(ctx context.Context, luid string) (definition.Definition, error) {
	item, err := a.Client.GetDefinition(ctx, luid)
	return DefinitionObservation(item), err
}

func (a DefinitionMutationPort) DeleteDefinition(ctx context.Context, luid string) (definition.DeleteResult, error) {
	item, err := a.Client.DeleteDefinition(ctx, luid)
	return definition.DeleteResult{Status: item.Status, DefinitionLUID: item.LUID, HTTPStatus: item.HTTPStatus, TableauRequestID: item.TableauRequestID}, err
}

func (a *DefinitionMutationPort) CreateDefinition(ctx context.Context, request definition.CreateRequest) (definition.CreateResult, error) {
	result, err := a.Client.CreateDefinition(ctx, request)
	return definition.CreateResult{Status: result.Status, DefinitionLUID: result.DefinitionLUID, DefaultMetricLUID: result.DefaultMetricLUID, DefaultMetricStatus: result.DefaultMetricStatus, TableauRequestID: result.TableauRequestID, PollRequestID: result.PollRequestID}, err
}

type DefinitionFieldPort struct {
	Schema DefinitionSchemaReader
}

type DefinitionSchemaReader interface {
	ReadDatasourceSchema(context.Context, string) (fieldcatalog.Schema, error)
}

func (v *DefinitionFieldPort) ResolveDefinitionFields(ctx context.Context, references definition.CreateFieldReferences) (definition.CreateFieldReferences, error) {
	schema, err := v.Schema.ReadDatasourceSchema(ctx, references.DatasourceLUID)
	if err != nil {
		return references, err
	}
	fields := make(map[string][]fieldcatalog.Field, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.ID] = append(fields[field.ID], field)
	}
	selectors := make([]string, 0, 2+len(references.AllowedDimensions))
	selectors = append(selectors, references.MeasureField, references.TimeDimension)
	selectors = append(selectors, references.AllowedDimensions...)
	resolved, err := fieldcatalog.ResolveFields(schema.Fields, selectors)
	if err != nil {
		return references, err
	}
	references.MeasureField, references.TimeDimension = resolved[0].ID, resolved[1].ID
	references.AllowedDimensions = make([]string, len(resolved)-2)
	for i, field := range resolved[2:] {
		references.AllowedDimensions[i] = field.ID
	}
	return references, v.validateFields(fields, references)
}

func (v *DefinitionFieldPort) validateFields(fields map[string][]fieldcatalog.Field, references definition.CreateFieldReferences) error {
	aggregation := strings.ToUpper(strings.TrimSpace(references.Aggregation))
	if aggregation == "" {
		aggregation = "AGGREGATION_SUM"
	}
	measureRoles := []string{"measure"}
	if aggregation == "AGGREGATION_COUNT" || aggregation == "AGGREGATION_COUNT_DISTINCT" {
		measureRoles = append(measureRoles, "dimension")
	}
	measure, err := exactPulseField(fields, references.MeasureField, measureRoles...)
	if err != nil {
		return fmt.Errorf("measure field: %w", err)
	}
	if _, err := exactPulseField(fields, references.TimeDimension, "date"); err != nil {
		return fmt.Errorf("time dimension: %w", err)
	}
	for _, fieldID := range references.AllowedDimensions {
		if _, err := exactPulseField(fields, fieldID, "dimension"); err != nil {
			return fmt.Errorf("allowed dimension %q: %w", fieldID, err)
		}
	}
	if measure.RequiresUserAggregation && aggregation != "AGGREGATION_USER" {
		return fmt.Errorf("field %q is already aggregated; use --aggregation USER", measure.ID)
	}
	if !measure.RequiresUserAggregation && aggregation == "AGGREGATION_USER" {
		return fmt.Errorf("field %q does not support USER aggregation", measure.ID)
	}
	if (aggregation == "AGGREGATION_SUM" || aggregation == "AGGREGATION_AVERAGE") && !numericPulseType(measure.DataType) {
		return fmt.Errorf("field %q has nonnumeric datatype %q for %s", measure.ID, measure.DataType, aggregation)
	}
	return nil
}

func exactPulseField(fields map[string][]fieldcatalog.Field, id string, roles ...string) (fieldcatalog.Field, error) {
	id = strings.TrimSpace(id)
	matches := fields[id]
	if len(matches) != 1 {
		return fieldcatalog.Field{}, fmt.Errorf("exact field ID %q matched %d fields", id, len(matches))
	}
	field := matches[0]
	if field.Excluded || field.Role == "excluded" {
		if field.ExclusionReason == "table_calc" && len(roles) > 0 && roles[0] == "measure" {
			return fieldcatalog.Field{}, fmt.Errorf("field %q is a table calculation; table calculations cannot be used as Pulse measures", id)
		}
		return fieldcatalog.Field{}, fmt.Errorf("field %q is excluded: %s", id, field.ExclusionReason)
	}
	for _, role := range roles {
		if field.Role == role {
			return field, nil
		}
	}
	return fieldcatalog.Field{}, fmt.Errorf("field %q has role %q, expected %q", id, field.Role, strings.Join(roles, " or "))
}

func numericPulseType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "NUMBER", "INTEGER", "INT", "LONG", "REAL", "FLOAT", "DOUBLE", "DECIMAL", "NUMERIC", "CURRENCY":
		return true
	default:
		return false
	}
}
