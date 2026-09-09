package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type Reader interface {
	ReadBundle(context.Context, string) (Bundle, error)
}
type Validator interface {
	ValidateDefinition(context.Context, json.RawMessage, []Metric) error
}
type Writer interface {
	CreateDefinition(context.Context, json.RawMessage) (DefinitionResult, error)
	CreateMetric(context.Context, string, json.RawMessage) (MetricResult, error)
	VerifyMetric(context.Context, string, string, string, string, json.RawMessage) error
}
type Action struct {
	reader    Reader
	validator Validator
	writer    Writer
}

func New(reader Reader, validator Validator, writer Writer) *Action {
	return &Action{reader: reader, validator: validator, writer: writer}
}

func ValidateInput(input Input) error {
	selectors := 0
	for _, value := range []string{input.Artifact, input.ArtifactID, input.ArtifactName} {
		if strings.TrimSpace(value) != "" {
			selectors++
		}
	}
	if strings.TrimSpace(input.Environment) == "" || selectors != 1 {
		return failure(input, "usage", errs.KindUsage, "Pulse bundle publish requires a target environment and exactly one artifact selector: --id, --artifact-name, or --artifact.", nil)
	}
	if len(input.DatasourceMap) == 0 || len(input.DatasourceMap) > 100 {
		return failure(input, "usage", errs.KindUsage, "Provide an explicit --datasource-map source=destination for every bundle datasource, including same-site publishing.", nil)
	}
	_, err := datasourceMappings(input.DatasourceMap)
	if err != nil {
		return failure(input, "usage", errs.KindUsage, "Invalid explicit datasource mapping.", err)
	}
	return nil
}

func datasourceMappings(values []string) (map[string]string, error) {
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

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil || a.validator == nil || a.writer == nil {
		return Output{}, failure(input, "unconfigured", errs.KindRuntime, "Pulse bundle publish is not configured.", nil)
	}
	bundle, err := a.reader.ReadBundle(ctx, input.Artifact)
	if err != nil {
		return Output{}, failure(input, "artifact", errs.KindUsage, "Portable Pulse bundle validation failed.", err)
	}
	plan, err := PrepareBundle(input, bundle)
	if err != nil {
		return Output{}, err
	}
	document, destination := plan.DefinitionConfiguration, plan.DestinationDatasourceLUID
	bundle.Metrics = plan.Metrics
	output := Output{Status: "preview", Plan: plan, Mappings: []Mapping{}, Complete: true, Help: []string{"Publishing creates new Pulse objects; existing definitions and metrics are never overwritten. Followers, users, values, and generated insights are not copied."}}
	if err := a.validator.ValidateDefinition(ctx, document, bundle.Metrics); err != nil {
		output.Status = "invalid"
		output.Complete = false
		return output, failure(input, "validation", errs.KindOperation, "Destination datasource field validation failed; no objects were created.", err)
	}
	if input.Preview {
		return output, nil
	}
	if err := a.validator.ValidateDefinition(ctx, document, bundle.Metrics); err != nil {
		output.Status = "invalid"
		output.Complete = false
		return output, failure(input, "revalidation", errs.KindOperation, "Destination datasource field revalidation failed; no objects were created.", err)
	}
	output.Status = "failed"
	output.Complete = false
	created, err := a.writer.CreateDefinition(ctx, document)
	if created.LUID != "" {
		output.Mappings = append(output.Mappings, Mapping{Kind: "definition", SourceLUID: bundle.DefinitionLUID, DestinationLUID: created.LUID})
		output.Status = "partial"
		output.Help = []string{commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", created.LUID)}
	}
	if err != nil {
		return output, failure(input, "create", errs.KindOperation, "Pulse definition creation or default-metric resolution failed. Inspect confirmed identities before retrying; publishing again creates another definition.", err)
	}
	if created.LUID == "" || created.LUID == bundle.DefinitionLUID {
		return output, failure(input, "identity", errs.KindOperation, "The provider did not return a new authoritative Pulse definition identity.", nil)
	}
	for _, metric := range bundle.Metrics {
		result, err := a.writer.CreateMetric(ctx, created.LUID, metric.Specification)
		if result.LUID != "" {
			output.Mappings = append(output.Mappings, Mapping{Kind: "metric", SourceLUID: metric.LUID, DestinationLUID: result.LUID})
		}
		if err != nil {
			return output, failure(input, "metric_create", errs.KindOperation, "Pulse bundle publish stopped after a metric creation failure. Confirmed mappings are retained; do not blindly republish.", err)
		}
		if result.LUID == "" || result.LUID == metric.LUID {
			return output, failure(input, "metric_identity", errs.KindOperation, "The provider did not return a new authoritative Pulse metric identity.", nil)
		}
		if metric.IsDefault && result.LUID != created.DefaultMetricLUID {
			return output, failure(input, "default_metric", errs.KindOperation, "The source default specification did not match the new definition's default metric. Confirmed mappings are retained; existing objects were not overwritten.", nil)
		}
		if err := a.writer.VerifyMetric(ctx, result.LUID, created.LUID, destination, input.SiteLUID, metric.Specification); err != nil {
			return output, failure(input, "reconciliation", errs.KindOperation, "A recreated metric could not be verified against its complete saved specification. Confirmed mappings are retained.", err)
		}
	}
	output.Status = "published"
	output.Complete = true
	return output, nil
}

// PrepareBundle performs local payload and mapping checks before authentication.
func PrepareBundle(input Input, bundle Bundle) (Plan, error) {
	if err := ValidateInput(input); err != nil {
		return Plan{}, err
	}
	mappings, _ := datasourceMappings(input.DatasourceMap)
	destination, ok := mappings[bundle.DatasourceLUID]
	if !ok || len(mappings) != 1 {
		return Plan{}, failure(input, "mapping", errs.KindUsage, "Every bundle datasource requires exactly one explicit mapping; unrelated mappings are not accepted.", nil)
	}
	document, name, err := createDocument(bundle.Configuration, destination)
	if err != nil {
		return Plan{}, failure(input, "configuration", errs.KindUsage, "Pulse bundle definition cannot be recreated without losing configuration.", err)
	}
	if len(bundle.Metrics) == 0 || len(bundle.Metrics) > 10000 {
		return Plan{}, failure(input, "metrics", errs.KindUsage, "Pulse bundle requires a complete bounded metric inventory.", nil)
	}
	metrics := make([]Metric, len(bundle.Metrics))
	seen := map[string]bool{}
	for i, metric := range bundle.Metrics {
		if metric.LUID == "" || seen[metric.LUID] {
			return Plan{}, failure(input, "metric_identity", errs.KindUsage, "Metric identities must be nonempty and unique.", nil)
		}
		seen[metric.LUID] = true
		var specification map[string]json.RawMessage
		if json.Unmarshal(metric.Specification, &specification) != nil || specification == nil {
			return Plan{}, failure(input, "metric_specification", errs.KindUsage, "Metric specification must be complete.", nil)
		}
		if raw, exists := specification["datasource"]; exists {
			var datasource map[string]json.RawMessage
			var id string
			if json.Unmarshal(raw, &datasource) != nil || json.Unmarshal(datasource["id"], &id) != nil || id != bundle.DatasourceLUID {
				return Plan{}, failure(input, "metric_datasource", errs.KindUsage, "Metric datasource does not match the explicitly mapped source.", nil)
			}
			datasource["id"], _ = json.Marshal(destination)
			specification["datasource"], _ = json.Marshal(datasource)
		}
		data, err := json.Marshal(specification)
		if err != nil {
			return Plan{}, err
		}
		metrics[i] = Metric{LUID: metric.LUID, IsDefault: metric.IsDefault, Specification: data}
	}
	return Plan{Environment: input.Environment, Site: input.Site, Artifact: input.Artifact, SourceDefinitionLUID: bundle.DefinitionLUID, Name: name, SourceDatasourceLUID: bundle.DatasourceLUID, DestinationDatasourceLUID: destination, NewObjectsOnly: true, MetricCount: len(metrics), DefinitionConfiguration: document, Metrics: metrics, ReviewComplete: true}, nil
}

func createDocument(configuration []byte, destination string) (json.RawMessage, string, error) {
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

func failure(input Input, id string, kind errs.Kind, summary string, cause error) error {
	return &errs.Error{ID: "pulse.definition.publish." + id, Kind: kind, Operation: "pulse.definition.publish", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, TableauRequestID: errs.TableauRequestID(cause), Retryable: errs.Bool(false), CorrectiveAction: "Inspect any confirmed destination identities. Correct the bundle or explicit datasource mapping and preview before publishing; never blindly retry an uncertain create."}
}
