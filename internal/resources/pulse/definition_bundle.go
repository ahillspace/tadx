package pulse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type DefinitionBundleArtifactPort struct{}

func (DefinitionBundleArtifactPort) LoadBundle(ctx context.Context, root, workspaceName string, input pulsedefinition.PublishInput) (pulsedefinition.PublishInput, pulsedefinition.PublishBundle, error) {
	managed, err := artifact.Resolve(ctx, root, artifact.Selector{Kind: "pulse-definition", Path: input.Artifact, LUID: input.ArtifactID, Name: input.ArtifactName})
	if err != nil {
		if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
			return input, pulsedefinition.PublishBundle{}, artifact.MapResolutionError("pulse.definition.publish", workspaceName, input.ArtifactID, err)
		}
		return input, pulsedefinition.PublishBundle{}, definitionBundleSetupError("pulse.definition.publish.artifact", input.Environment, "Pulse bundle artifact resolution failed.", "Select an exact workspace-relative Pulse artifact.", err)
	}
	bundle, err := artifact.ReadPulseBundle(ctx, filepath.Join(root, filepath.FromSlash(managed.Path)))
	if err != nil {
		return input, pulsedefinition.PublishBundle{}, definitionBundleSetupError("pulse.definition.publish.bundle", input.Environment, "Portable Pulse bundle validation failed.", "Pull a complete Pulse bundle before publishing.", err)
	}
	input.Artifact = managed.Path
	input.ArtifactID, input.ArtifactName = "", ""
	input.WorkspaceName = workspaceName
	loaded := pulsedefinition.PublishBundle{DefinitionLUID: bundle.DefinitionLUID, DatasourceLUID: bundle.DatasourceReferences[0], Configuration: bundle.Definition, Metrics: make([]pulsedefinition.PublishMetric, len(bundle.Metrics))}
	for i, metric := range bundle.Metrics {
		loaded.Metrics[i] = pulsedefinition.PublishMetric{LUID: metric.LUID, IsDefault: metric.IsDefault, Specification: metric.Specification}
	}
	return input, loaded, nil
}

func definitionBundleSetupError(id, environment, summary, corrective string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, corrective)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "pulse.definition.publish", Environment: environment, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}

type DefinitionBundlePort struct {
	Client *tableaupulse.Client
	Schema DefinitionSchemaReader
}

func (a DefinitionBundlePort) ValidateDefinition(ctx context.Context, data json.RawMessage, metrics []pulsedefinition.PublishMetric) error {
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
	schema, err := a.Schema.ReadDatasourceSchema(ctx, request.Specification.Datasource.ID)
	if err != nil {
		return err
	}
	fields := map[string][]fieldcatalog.Field{}
	for _, field := range schema.Fields {
		fields[field.ID] = append(fields[field.ID], field)
	}
	validator := DefinitionFieldPort{}
	if err := validator.validateFields(fields, pulsedefinition.CreateFieldReferences{DatasourceLUID: request.Specification.Datasource.ID, MeasureField: request.Specification.BasicSpecification.Measure.Field, Aggregation: request.Specification.BasicSpecification.Measure.Aggregation, TimeDimension: request.Specification.BasicSpecification.TimeDimension.Field, AllowedDimensions: request.ExtensionOptions.AllowedDimensions}); err != nil {
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

func (a DefinitionBundlePort) CreateDefinition(ctx context.Context, data json.RawMessage) (pulsedefinition.PublishDefinitionResult, error) {
	result, err := a.Client.CreateDefinitionDocument(ctx, data)
	return pulsedefinition.PublishDefinitionResult{LUID: result.DefinitionLUID, DefaultMetricLUID: result.DefaultMetricLUID, RequestID: result.TableauRequestID}, err
}

func pulseBundleSpecification(data json.RawMessage) (map[string]any, error) {
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err := decoder.Decode(&result)
	return result, err
}

func (a DefinitionBundlePort) CreateMetric(ctx context.Context, definition string, data json.RawMessage) (pulsedefinition.PublishMetricResult, error) {
	specification, err := pulseBundleSpecification(data)
	if err != nil {
		return pulsedefinition.PublishMetricResult{}, err
	}
	result, err := a.Client.GetOrCreateMetric(ctx, tableaupulse.GetOrCreateRequest{DefinitionLUID: definition, Specification: specification})
	return pulsedefinition.PublishMetricResult{LUID: result.MetricLUID, RequestID: result.TableauRequestID}, err
}

func (a DefinitionBundlePort) VerifyMetric(ctx context.Context, metric, definition, datasource, site string, data json.RawMessage) error {
	specification, err := pulseBundleSpecification(data)
	if err != nil {
		return err
	}
	result, err := a.Client.ReconcileBundleMetric(ctx, tableaupulse.ExpectedMetric{MetricLUID: metric, DefinitionLUID: definition, DatasourceLUID: datasource, SiteLUID: site, Specification: specification})
	if err != nil {
		return err
	}
	if !result.SpecificationVerified || !result.OwnershipVerified {
		return fmt.Errorf("metric readback is not verified: %s", result.Status)
	}
	return nil
}

func (a DefinitionBundlePort) VerifyDefinition(ctx context.Context, definition, datasource, site string, data json.RawMessage) error {
	return a.Client.VerifyBundleDefinition(ctx, definition, datasource, site, data)
}
