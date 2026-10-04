package pulse

import (
	"context"
	"fmt"

	metric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type MetricMutationPort struct {
	*MetricInspectPort
	Client *tableaupulse.Client
	Schema DefinitionSchemaReader
}

func (a *MetricMutationPort) ResolveFilterFields(ctx context.Context, datasource string, selectors []string) ([]string, error) {
	schema, err := a.Schema.ReadDatasourceSchema(ctx, datasource)
	if err != nil {
		return nil, err
	}
	fields, err := fieldcatalog.ResolveFields(schema.Fields, selectors)
	if err != nil {
		return nil, err
	}
	resolved := make([]string, len(fields))
	for i, field := range fields {
		if field.Excluded || field.Role != "dimension" {
			return nil, fmt.Errorf("field %q is not an eligible Pulse dimension", field.ID)
		}
		resolved[i] = field.ID
	}
	return resolved, nil
}

func (a *MetricMutationPort) GetDefinition(ctx context.Context, luid string) (metric.ForkDefinition, error) {
	item, err := a.Client.GetDefinition(ctx, luid)
	if err != nil {
		return metric.ForkDefinition{}, err
	}
	return metric.ForkDefinition{LUID: item.LUID, DatasourceLUID: item.DatasourceLUID, AllowedDimensions: append([]string(nil), item.AllowedDimensions...), AllowedGranularities: append([]string(nil), item.AllowedGranularities...), FixedFilters: append([]any(nil), item.FixedFilters...), FixedFiltersKnown: item.FixedFiltersKnown}, nil
}

func (a *MetricMutationPort) GetOrCreateMetric(ctx context.Context, request metric.ForkCreateRequest) (metric.ForkCreateResult, error) {
	result, err := a.Client.GetOrCreateMetric(ctx, tableaupulse.GetOrCreateRequest{DefinitionLUID: request.DefinitionLUID, Specification: request.Specification})
	return metric.ForkCreateResult{MetricLUID: result.MetricLUID, MetricName: result.MetricName, Created: result.Created, RequestID: result.TableauRequestID}, err
}

func (a *MetricMutationPort) ReconcileMetric(ctx context.Context, expected metric.ForkExpectedMetric) (metric.ForkReconciliation, error) {
	result, err := a.Client.ReconcileMetric(ctx, tableaupulse.ExpectedMetric{MetricLUID: expected.MetricLUID, DefinitionLUID: expected.DefinitionLUID, DatasourceLUID: expected.DatasourceLUID, SiteLUID: expected.SiteLUID, Specification: expected.Specification})
	return metric.ForkReconciliation{Status: result.Status, Attempts: result.Attempts, OwnershipVerified: result.OwnershipVerified, RequestID: result.TableauRequestID, SpecificationVerified: result.SpecificationVerified, SavedSpecification: result.Metric.Specification, MetricRequestID: result.Metric.TableauRequestID, DefinitionRequestID: result.Definition.TableauRequestID, SavedDefinition: metric.ForkSavedDefinition{LUID: result.Definition.LUID, Name: result.Definition.Name, DatasourceLUID: result.Definition.DatasourceLUID}}, err
}
