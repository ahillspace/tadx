package definition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

type PublishReader interface {
	ReadBundle(context.Context, string) (PublishBundle, error)
}
type PublishValidator interface {
	ValidateDefinition(context.Context, json.RawMessage, []PublishMetric) error
}
type PublishWriter interface {
	CreateDefinition(context.Context, json.RawMessage) (PublishDefinitionResult, error)
	CreateMetric(context.Context, string, json.RawMessage) (PublishMetricResult, error)
	VerifyDefinition(context.Context, string, string, string, json.RawMessage) error
	VerifyMetric(context.Context, string, string, string, string, json.RawMessage) error
}

func PublishValidateInput(input PublishInput) error {
	selectors := 0
	for _, value := range []string{input.Artifact, input.ArtifactID, input.ArtifactName} {
		if strings.TrimSpace(value) != "" {
			selectors++
		}
	}
	if strings.TrimSpace(input.Environment) == "" || selectors != 1 {
		return publishFailure(input, "usage", errs.KindUsage, "Pulse bundle publish requires a target environment and exactly one artifact selector: --id, --artifact-name, or --artifact.", nil)
	}
	if len(input.DatasourceMap) == 0 || len(input.DatasourceMap) > 100 {
		return publishFailure(input, "usage", errs.KindUsage, "Provide an explicit --datasource-map source=destination for every bundle datasource, including same-site publishing.", nil)
	}
	_, err := publishDatasourceMappings(input.DatasourceMap)
	if err != nil {
		return publishFailure(input, "usage", errs.KindUsage, "Invalid explicit datasource mapping.", err)
	}
	return nil
}

func publishDatasourceMappings(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, value := range values {
		source, destination, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(source) == "" || strings.TrimSpace(destination) == "" || strings.Contains(destination, "=") {
			return nil, errors.New("mapping must use exact source=destination datasource LUIDs")
		}
		if _, exists := result[source]; exists {
			return nil, fmt.Errorf("duplicate datasource mapping for %q", source)
		}
		result[source] = destination
	}
	return result, nil
}

func Publish(ctx context.Context, reader PublishReader, validator PublishValidator, writer PublishWriter, input PublishInput) (PublishOutput, error) {
	if err := PublishValidateInput(input); err != nil {
		return PublishOutput{}, err
	}
	if reader == nil || validator == nil || writer == nil {
		return PublishOutput{}, publishFailure(input, "unconfigured", errs.KindRuntime, "Pulse bundle publish is not configured.", nil)
	}
	bundle, err := reader.ReadBundle(ctx, input.Artifact)
	if err != nil {
		return PublishOutput{}, publishFailure(input, "artifact", errs.KindUsage, "Portable Pulse bundle validation failed.", err)
	}
	plan, err := PublishPrepareBundle(input, bundle)
	if err != nil {
		return PublishOutput{}, err
	}
	document, destination := plan.DefinitionConfiguration, plan.DestinationDatasourceLUID
	bundle.Metrics = plan.Metrics
	output := PublishOutput{Status: "preview", Plan: plan, Mappings: []PublishMapping{}, Complete: true, Help: []string{"Publishing creates new Pulse objects; existing definitions and metrics are never overwritten. Followers, users, values, and generated insights are not copied."}}
	if err := validator.ValidateDefinition(ctx, document, bundle.Metrics); err != nil {
		output.Status = "invalid"
		output.Complete = false
		return output, publishFailureState(input, "validation", errs.KindOperation, "Destination datasource field validation failed; no objects were created.", err, errs.PhaseValidation, errs.OutcomeNotAttempted)
	}
	if input.Preview {
		return output, nil
	}
	if err := validator.ValidateDefinition(ctx, document, bundle.Metrics); err != nil {
		output.Status = "invalid"
		output.Complete = false
		return output, publishFailureState(input, "revalidation", errs.KindOperation, "Destination datasource field revalidation failed; no objects were created.", err, errs.PhaseValidation, errs.OutcomeNotAttempted)
	}
	output.Status = "failed"
	output.Complete = false
	created, err := writer.CreateDefinition(ctx, document)
	if created.LUID != "" {
		output.Mappings = append(output.Mappings, PublishMapping{Kind: "definition", SourceLUID: bundle.DefinitionLUID, DestinationLUID: created.LUID})
		output.Status = "partial"
		output.Help = []string{commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", created.LUID)}
	}
	if err != nil {
		outcome := errs.OutcomeUnknown
		if created.LUID != "" {
			outcome = errs.OutcomeConfirmed
		}
		return output, publishFailureState(input, "create", errs.KindOperation, "Pulse definition creation or default-metric resolution failed. Inspect confirmed identities before retrying; publishing again creates another definition.", err, errs.PhaseSubmission, outcome)
	}
	if created.LUID == "" || created.LUID == bundle.DefinitionLUID {
		return output, publishFailureState(input, "identity", errs.KindOperation, "The provider did not return a new authoritative Pulse definition identity.", nil, errs.PhaseSubmission, errs.OutcomeUnknown)
	}
	if err := writer.VerifyDefinition(ctx, created.LUID, destination, input.SiteLUID, document); err != nil {
		return output, publishFailureState(input, "definition_reconciliation", errs.KindOperation, "The created definition could not be verified against its submitted configuration. Confirmed identity is retained; no bundle metrics were requested.", err, errs.PhaseVerification, errs.OutcomeConfirmed)
	}
	for _, metric := range bundle.Metrics {
		result, err := writer.CreateMetric(ctx, created.LUID, metric.Specification)
		if result.LUID != "" {
			output.Mappings = append(output.Mappings, PublishMapping{Kind: "metric", SourceLUID: metric.LUID, DestinationLUID: result.LUID})
		}
		if err != nil {
			outcome := errs.OutcomeUnknown
			if result.LUID != "" {
				outcome = errs.OutcomeConfirmed
			}
			return output, publishFailureState(input, "metric_create", errs.KindOperation, "Pulse bundle publish stopped after a metric creation failure. Confirmed mappings are retained; do not blindly republish.", err, errs.PhaseSubmission, outcome)
		}
		if result.LUID == "" || result.LUID == metric.LUID {
			return output, publishFailureState(input, "metric_identity", errs.KindOperation, "The provider did not return a new authoritative Pulse metric identity.", nil, errs.PhaseSubmission, errs.OutcomeUnknown)
		}
		if metric.IsDefault && result.LUID != created.DefaultMetricLUID {
			return output, publishFailureState(input, "default_metric", errs.KindOperation, "The source default specification did not match the new definition's default metric. Confirmed mappings are retained; existing objects were not overwritten.", nil, errs.PhaseVerification, errs.OutcomeConfirmed)
		}
		if err := writer.VerifyMetric(ctx, result.LUID, created.LUID, destination, input.SiteLUID, metric.Specification); err != nil {
			return output, publishFailureState(input, "reconciliation", errs.KindOperation, "A recreated metric could not be verified against its complete saved specification. Confirmed mappings are retained.", err, errs.PhaseVerification, errs.OutcomeConfirmed)
		}
	}
	output.Status = "published"
	output.Complete = true
	return output, nil
}

// PrepareBundle performs local payload and mapping checks before authentication.
func PublishPrepareBundle(input PublishInput, bundle PublishBundle) (PublishPlan, error) {
	if err := PublishValidateInput(input); err != nil {
		return PublishPlan{}, err
	}
	mappings, _ := publishDatasourceMappings(input.DatasourceMap)
	destination, ok := mappings[bundle.DatasourceLUID]
	if !ok || len(mappings) != 1 {
		return PublishPlan{}, publishFailure(input, "mapping", errs.KindUsage, "Every bundle datasource requires exactly one explicit mapping; unrelated mappings are not accepted.", nil)
	}
	document, name, err := publishCreateDocument(bundle.Configuration, destination)
	if err != nil {
		return PublishPlan{}, publishFailure(input, "configuration", errs.KindUsage, "Pulse bundle definition cannot be recreated without losing configuration.", err)
	}
	if err := pulsecontract.ValidateDefinition(document); err != nil {
		return PublishPlan{}, publishFailure(input, "configuration", errs.KindUsage, "Pulse bundle definition configuration is invalid.", err)
	}
	if len(bundle.Metrics) == 0 || len(bundle.Metrics) > 10000 {
		return PublishPlan{}, publishFailure(input, "metrics", errs.KindUsage, "Pulse bundle requires a complete bounded metric inventory.", nil)
	}
	metrics := make([]PublishMetric, len(bundle.Metrics))
	seen := map[string]bool{}
	for i, metric := range bundle.Metrics {
		if metric.LUID == "" || seen[metric.LUID] {
			return PublishPlan{}, publishFailure(input, "metric_identity", errs.KindUsage, "Metric identities must be nonempty and unique.", nil)
		}
		seen[metric.LUID] = true
		var specification map[string]json.RawMessage
		if json.Unmarshal(metric.Specification, &specification) != nil || specification == nil {
			return PublishPlan{}, publishFailure(input, "metric_specification", errs.KindUsage, "Metric specification must be complete.", nil)
		}
		if raw, exists := specification["datasource"]; exists {
			var datasource map[string]json.RawMessage
			var id string
			if json.Unmarshal(raw, &datasource) != nil || json.Unmarshal(datasource["id"], &id) != nil || id != bundle.DatasourceLUID {
				return PublishPlan{}, publishFailure(input, "metric_datasource", errs.KindUsage, "Metric datasource does not match the explicitly mapped source.", nil)
			}
			datasource["id"], _ = json.Marshal(destination)
			specification["datasource"], _ = json.Marshal(datasource)
		}
		data, err := json.Marshal(specification)
		if err != nil {
			return PublishPlan{}, err
		}
		if err := pulsecontract.ValidateMetric(data); err != nil {
			return PublishPlan{}, publishFailure(input, "metric_specification", errs.KindUsage, "Pulse bundle metric specification is invalid.", err)
		}
		metrics[i] = PublishMetric{LUID: metric.LUID, IsDefault: metric.IsDefault, Specification: data}
	}
	return PublishPlan{Environment: input.Environment, Site: input.Site, Artifact: input.Artifact, SourceDefinitionLUID: bundle.DefinitionLUID, Name: name, SourceDatasourceLUID: bundle.DatasourceLUID, DestinationDatasourceLUID: destination, NewObjectsOnly: true, MetricCount: len(metrics), DefinitionConfiguration: document, Metrics: metrics, ReviewComplete: true}, nil
}

func publishCreateDocument(configuration []byte, destination string) (json.RawMessage, string, error) {
	var source map[string]json.RawMessage
	if json.Unmarshal(configuration, &source) != nil || source == nil {
		return nil, "", errors.New("definition requires a JSON object")
	}
	var metadata struct{ ID, Name, Description string }
	if json.Unmarshal(source["metadata"], &metadata) != nil || metadata.ID == "" || metadata.Name == "" {
		return nil, "", errors.New("definition metadata identity is incomplete")
	}
	allowed := map[string]bool{"metadata": true, "specification": true, "extension_options": true, "representation_options": true, "insights_options": true, "comparisons": true, "datasource_goals": true, "related_links": true, "certification": true}
	for key := range source {
		if !allowed[key] {
			return nil, "", fmt.Errorf("unsupported source definition section %q; no field was silently discarded", key)
		}
	}
	delete(source, "metadata")
	source["name"], _ = json.Marshal(metadata.Name)
	source["description"], _ = json.Marshal(metadata.Description)
	var specification map[string]json.RawMessage
	if json.Unmarshal(source["specification"], &specification) != nil || specification == nil {
		return nil, "", errors.New("definition specification is missing")
	}
	var datasource map[string]json.RawMessage
	if json.Unmarshal(specification["datasource"], &datasource) != nil || datasource == nil {
		return nil, "", errors.New("definition datasource is missing")
	}
	datasource["id"], _ = json.Marshal(destination)
	specification["datasource"], _ = json.Marshal(datasource)
	source["specification"], _ = json.Marshal(specification)
	data, err := json.Marshal(source)
	if err == nil && bytes.Equal(data, []byte("null")) {
		err = errors.New("empty create document")
	}
	return data, metadata.Name, err
}

func publishFailure(input PublishInput, id string, kind errs.Kind, summary string, cause error) error {
	phase := errs.PhaseValidation
	if kind == errs.KindRuntime {
		phase = errs.PhaseSetup
	}
	return &errs.Error{ID: "pulse.definition.publish." + id, Kind: kind, Operation: "pulse.definition.publish", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, TableauRequestID: errs.TableauRequestID(cause), Retryable: errs.Bool(false), CorrectiveAction: "Inspect any confirmed destination identities. Correct the bundle or explicit datasource mapping and preview before publishing; never blindly retry an uncertain create.", Phase: phase, Outcome: errs.OutcomeNotAttempted}
}

func publishFailureState(input PublishInput, id string, kind errs.Kind, summary string, cause error, phase errs.Phase, outcome errs.Outcome) error {
	result := publishFailure(input, id, kind, summary, cause).(*errs.Error)
	result.Phase = phase
	result.Outcome = outcome
	return result
}

type PublishInput struct {
	Environment   string
	Site          string
	SiteLUID      string
	Workspace     string
	WorkspaceName string
	Artifact      string
	ArtifactID    string
	ArtifactName  string
	DatasourceMap []string
	Preview       bool
}

type PublishBundle struct {
	DefinitionLUID string
	DatasourceLUID string
	Configuration  json.RawMessage
	Metrics        []PublishMetric
}

type PublishMetric struct {
	LUID          string          `json:"source_metric_luid"`
	IsDefault     bool            `json:"is_default"`
	Specification json.RawMessage `json:"specification"`
}

type PublishMapping struct {
	Kind            string `json:"kind"`
	SourceLUID      string `json:"source_luid"`
	DestinationLUID string `json:"destination_luid"`
}

type PublishPlan struct {
	Environment               string          `json:"environment"`
	Site                      string          `json:"site"`
	Artifact                  string          `json:"artifact"`
	SourceDefinitionLUID      string          `json:"source_definition_luid"`
	Name                      string          `json:"name"`
	SourceDatasourceLUID      string          `json:"source_datasource_luid"`
	DestinationDatasourceLUID string          `json:"destination_datasource_luid"`
	NewObjectsOnly            bool            `json:"new_objects_only"`
	MetricCount               int             `json:"metric_count"`
	DefinitionConfiguration   json.RawMessage `json:"definition_configuration,omitempty"`
	Metrics                   []PublishMetric `json:"metrics"`
	ReviewComplete            bool            `json:"review_complete"`
}

type PublishDefinitionResult struct{ LUID, DefaultMetricLUID, RequestID string }
type PublishMetricResult struct{ LUID, RequestID string }

type PublishOutput struct {
	Status   string           `json:"status"`
	Plan     PublishPlan      `json:"plan"`
	Mappings []PublishMapping `json:"mappings"`
	Complete bool             `json:"complete"`
	Warnings []string         `json:"warnings,omitempty"`
	Help     []string         `json:"help"`
}

type publishCompactResult struct {
	Status   string           `json:"status"`
	Plan     PublishPlan      `json:"plan"`
	Mappings []PublishMapping `json:"mappings"`
	Complete bool             `json:"complete"`
	Warnings []string         `json:"warnings,omitempty"`
	Details  string           `json:"details"`
	Help     []string         `json:"help"`
}

func (o PublishOutput) CompactOutput() any {
	plan := o.Plan
	if len(plan.Metrics) > 10 {
		plan.Metrics = plan.Metrics[:10]
		plan.ReviewComplete = false
	}
	if len(plan.DefinitionConfiguration) > 16384 {
		plan.DefinitionConfiguration = nil
		plan.ReviewComplete = false
	}
	metrics := make([]PublishMetric, len(plan.Metrics))
	copy(metrics, plan.Metrics)
	for i := range metrics {
		if len(metrics[i].Specification) > 4096 {
			metrics[i].Specification = nil
			plan.ReviewComplete = false
		}
	}
	plan.Metrics = metrics
	mappings := o.Mappings
	if len(mappings) > 20 {
		mappings = mappings[:20]
		plan.ReviewComplete = false
	}
	return publishCompactResult{Status: o.Status, Plan: plan, Mappings: mappings, Complete: o.Complete, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}
func (o PublishOutput) FullOutput() any { return o }
