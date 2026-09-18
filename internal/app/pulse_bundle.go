package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	definitionpublish "github.com/ahillspace/tadx/actions/pulse/definition/publish"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func (c *pulseCommands) PublishPulseDefinition(ctx context.Context, input definitionpublish.Input) (definitionpublish.Output, error) {
	if err := definitionpublish.ValidateInput(input); err != nil {
		return definitionpublish.Output{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return definitionpublish.Output{}, capabilitySetupError("pulse.definition.publish.workspace", "pulse.definition.publish", input.Environment, "", "Pulse bundle workspace resolution failed.", "Select an exact logical workspace.", err)
	}
	managed, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: "pulse-definition", Path: input.Artifact, LUID: input.ArtifactID, Name: input.ArtifactName})
	if err != nil {
		if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
			return definitionpublish.Output{}, mapArtifactResolutionError("pulse.definition.publish", workspace.Name, input.ArtifactID, err)
		}
		return definitionpublish.Output{}, capabilitySetupError("pulse.definition.publish.artifact", "pulse.definition.publish", input.Environment, "", "Pulse bundle artifact resolution failed.", "Select an exact workspace-relative Pulse artifact.", err)
	}
	bundle, err := artifact.ReadPulseBundle(ctx, filepath.Join(workspace.Root, filepath.FromSlash(managed.Path)))
	if err != nil {
		return definitionpublish.Output{}, capabilitySetupError("pulse.definition.publish.bundle", "pulse.definition.publish", input.Environment, "", "Portable Pulse bundle validation failed.", "Pull a complete Pulse bundle before publishing.", err)
	}
	input.Artifact = managed.Path
	input.ArtifactID, input.ArtifactName = "", ""
	input.WorkspaceName = workspace.Name
	reader := pulseBundleReader{bundle: definitionpublish.Bundle{DefinitionLUID: bundle.DefinitionLUID, DatasourceLUID: bundle.DatasourceReferences[0], Configuration: bundle.Definition, Metrics: make([]definitionpublish.Metric, len(bundle.Metrics))}}
	for i, metric := range bundle.Metrics {
		reader.bundle.Metrics[i] = definitionpublish.Metric{LUID: metric.LUID, IsDefault: metric.IsDefault, Specification: metric.Specification}
	}
	if _, err := definitionpublish.PrepareBundle(input, reader.bundle); err != nil {
		return definitionpublish.Output{}, err
	}
	connection, err := c.connect(ctx, input.Environment, true)
	if err != nil {
		return definitionpublish.Output{}, remoteSetupError("pulse.definition.publish", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site, input.SiteLUID = connection.environment.Alias, connection.environment.SiteContentURL, connection.siteLUID
	adapter := pulseBundleAdapter{connection: connection}
	return definitionpublish.New(reader, adapter, adapter).Execute(ctx, input)
}

type pulseBundleReader struct{ bundle definitionpublish.Bundle }

func (r pulseBundleReader) ReadBundle(context.Context, string) (definitionpublish.Bundle, error) {
	return r.bundle, nil
}

type pulseBundleAdapter struct{ connection pulseConnection }

func (a pulseBundleAdapter) ValidateDefinition(ctx context.Context, data json.RawMessage, metrics []definitionpublish.Metric) error {
	// The raw contract validates saved business sections before this boundary.
	// Decode only destination-validation inputs without narrowing unrelated wire shapes.
	var request struct {
		Specification struct {
			Datasource         tableaupulse.Datasource `json:"datasource"`
			BasicSpecification struct {
				Measure       tableaupulse.Measure       `json:"measure"`
				TimeDimension tableaupulse.TimeDimension `json:"time_dimension"`
				Filters       json.RawMessage            `json:"filters"`
			} `json:"basic_specification"`
		} `json:"specification"`
		ExtensionOptions struct {
			AllowedDimensions    []string `json:"allowed_dimensions"`
			AllowedGranularities []string `json:"allowed_granularities"`
		} `json:"extension_options"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}
	schema, err := a.connection.schema.ReadDatasourceSchema(ctx, request.Specification.Datasource.ID)
	if err != nil {
		return err
	}
	fields := map[string][]fieldcatalog.Field{}
	for _, field := range schema.Fields {
		fields[field.ID] = append(fields[field.ID], field)
	}
	validator := pulseDefinitionFieldValidator{}
	if err := validator.validateFields(fields, definitioncreate.FieldReferences{DatasourceLUID: request.Specification.Datasource.ID, MeasureField: request.Specification.BasicSpecification.Measure.Field, Aggregation: request.Specification.BasicSpecification.Measure.Aggregation, TimeDimension: request.Specification.BasicSpecification.TimeDimension.Field, AllowedDimensions: request.ExtensionOptions.AllowedDimensions}); err != nil {
		return err
	}
	if err := validatePulseBundleFilters(fields, request.Specification.BasicSpecification.Filters); err != nil {
		return fmt.Errorf("fixed filters: %w", err)
	}
	for _, metric := range metrics {
		var specification map[string]json.RawMessage
		if json.Unmarshal(metric.Specification, &specification) != nil || specification == nil {
			return errors.New("metric specification must be an object")
		}
		var period map[string]json.RawMessage
		if json.Unmarshal(specification["measurement_period"], &period) != nil || len(period) == 0 {
			return errors.New("metric measurement period must be complete")
		}
		if err := validatePulseBundleFilters(fields, specification["filters"]); err != nil {
			return fmt.Errorf("metric %q filters: %w", metric.LUID, err)
		}
		var granularity string
		if json.Unmarshal(period["granularity"], &granularity) != nil || granularity == "" {
			return errors.New("metric measurement period granularity is missing")
		}
		allowedGranularity := false
		for _, allowed := range request.ExtensionOptions.AllowedGranularities {
			if granularity == allowed {
				allowedGranularity = true
			}
		}
		if !allowedGranularity {
			return fmt.Errorf("metric %q granularity %q is not allowed by its saved definition", metric.LUID, granularity)
		}
		var filters []struct {
			Field string `json:"field"`
		}
		_ = json.Unmarshal(specification["filters"], &filters)
		for _, filter := range filters {
			allowed := false
			for _, dimension := range request.ExtensionOptions.AllowedDimensions {
				if filter.Field == dimension {
					allowed = true
				}
			}
			if !allowed {
				return fmt.Errorf("metric %q filter %q is not an adjustable dimension", metric.LUID, filter.Field)
			}
		}
	}
	return nil
}

func validatePulseBundleFilters(fields map[string][]fieldcatalog.Field, raw json.RawMessage) error {
	var filters []map[string]json.RawMessage
	if json.Unmarshal(raw, &filters) != nil || filters == nil {
		return errors.New("filters require an explicit complete array")
	}
	for _, filter := range filters {
		var id string
		if json.Unmarshal(filter["field"], &id) != nil || id == "" {
			return errors.New("filter field identity is missing")
		}
		if _, err := exactPulseField(fields, id, "dimension", "date"); err != nil {
			return err
		}
	}
	return nil
}

func (a pulseBundleAdapter) CreateDefinition(ctx context.Context, data json.RawMessage) (definitionpublish.DefinitionResult, error) {
	result, err := a.connection.client.CreateDefinitionDocument(ctx, data)
	return definitionpublish.DefinitionResult{LUID: result.DefinitionLUID, DefaultMetricLUID: result.DefaultMetricLUID, RequestID: result.TableauRequestID}, err
}

func pulseBundleSpecification(data json.RawMessage) (map[string]any, error) {
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err := decoder.Decode(&result)
	return result, err
}

func (a pulseBundleAdapter) CreateMetric(ctx context.Context, definition string, data json.RawMessage) (definitionpublish.MetricResult, error) {
	specification, err := pulseBundleSpecification(data)
	if err != nil {
		return definitionpublish.MetricResult{}, err
	}
	result, err := a.connection.client.GetOrCreateMetric(ctx, tableaupulse.GetOrCreateRequest{DefinitionLUID: definition, Specification: specification})
	return definitionpublish.MetricResult{LUID: result.MetricLUID, RequestID: result.TableauRequestID}, err
}

func (a pulseBundleAdapter) VerifyMetric(ctx context.Context, metric, definition, datasource, site string, data json.RawMessage) error {
	specification, err := pulseBundleSpecification(data)
	if err != nil {
		return err
	}
	result, err := a.connection.client.ReconcileBundleMetric(ctx, tableaupulse.ExpectedMetric{MetricLUID: metric, DefinitionLUID: definition, DatasourceLUID: datasource, SiteLUID: site, Specification: specification})
	if err != nil {
		return err
	}
	if !result.SpecificationVerified || !result.OwnershipVerified {
		return fmt.Errorf("metric readback is not verified: %s", result.Status)
	}
	return nil
}

func (a pulseBundleAdapter) VerifyDefinition(ctx context.Context, definition, datasource, site string, data json.RawMessage) error {
	return a.connection.client.VerifyBundleDefinition(ctx, definition, datasource, site, data)
}
