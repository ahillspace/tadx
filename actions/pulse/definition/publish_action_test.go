package definition_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/errs"
)

type publishDependencies struct {
	bundle                                       pulsedefinition.PublishBundle
	validations, writes                          int
	failValidation                               int
	failCreate, failMetric, failVerify           bool
	definitionVerifications, metricVerifications int
	failDefinitionVerify                         bool
	events                                       []string
}

func (d *publishDependencies) ValidateDefinition(context.Context, json.RawMessage, []pulsedefinition.PublishMetric) error {
	d.events = append(d.events, "validate")
	d.validations++
	if d.validations == d.failValidation {
		return errors.New("invalid destination field")
	}
	return nil
}
func (d *publishDependencies) CreateDefinition(context.Context, json.RawMessage) (pulsedefinition.PublishDefinitionResult, error) {
	d.events = append(d.events, "create definition")
	d.writes++
	result := pulsedefinition.PublishDefinitionResult{LUID: "new-definition"}
	if d.failCreate {
		return result, errors.New("default resolution failed")
	}
	return result, nil
}
func (d *publishDependencies) CreateMetric(context.Context, string, json.RawMessage) (pulsedefinition.PublishMetricResult, error) {
	d.events = append(d.events, "create metric")
	d.writes++
	result := pulsedefinition.PublishMetricResult{LUID: "new-metric"}
	if d.failMetric {
		return result, errors.New("metric response failed")
	}
	return result, nil
}
func (d *publishDependencies) VerifyMetric(context.Context, string, string, string, string, json.RawMessage) error {
	d.events = append(d.events, "verify metric")
	d.metricVerifications++
	if d.failVerify {
		return errors.New("specification mismatch")
	}
	return nil
}
func (d *publishDependencies) VerifyDefinition(context.Context, string, string, string, json.RawMessage) error {
	d.events = append(d.events, "verify definition")
	d.definitionVerifications++
	if d.failDefinitionVerify {
		return errors.New("saved definition configuration mismatch")
	}
	return nil
}

func TestPublishVerifiesSharedDefinitionOnceBeforeMetrics(t *testing.T) {
	d := &publishDependencies{bundle: publishBundleFixture()}
	d.bundle.Metrics = append(d.bundle.Metrics, pulsedefinition.PublishMetric{LUID: "metric-2", Specification: d.bundle.Metrics[0].Specification})
	output, err := publish(context.Background(), d, publishInputFixture())
	if err != nil || !output.Complete || d.definitionVerifications != 1 || d.metricVerifications != 2 {
		t.Fatalf("output=%#v err=%v definition checks=%d metric checks=%d", output, err, d.definitionVerifications, d.metricVerifications)
	}
	want := []string{"validate", "validate", "create definition", "verify definition", "create metric", "verify metric", "create metric", "verify metric"}
	if !reflect.DeepEqual(d.events, want) {
		t.Fatalf("events = %v; want %v", d.events, want)
	}
}

func TestPublishDefinitionMismatchRetainsIdentityAndStopsBeforeMetrics(t *testing.T) {
	d := &publishDependencies{bundle: publishBundleFixture(), failDefinitionVerify: true}
	output, err := publish(context.Background(), d, publishInputFixture())
	if err == nil || output.Complete || output.Status != "partial" || d.writes != 1 || d.metricVerifications != 0 || len(output.Mappings) != 1 || output.Mappings[0].DestinationLUID != "new-definition" {
		t.Fatalf("output=%#v err=%v writes=%d", output, err, d.writes)
	}
}
func publishBundleFixture() pulsedefinition.PublishBundle {
	return pulsedefinition.PublishBundle{DefinitionLUID: "definition-1", DatasourceLUID: "ds-1", Configuration: json.RawMessage(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"ds-1"},"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"},"filters":[]}},"extension_options":{"allowed_dimensions":[],"allowed_granularities":["GRANULARITY_BY_DAY"]}}`), Metrics: []pulsedefinition.PublishMetric{{LUID: "metric-1", Specification: json.RawMessage(`{"datasource":{"id":"ds-1"},"measurement_period":{"granularity":"GRANULARITY_BY_DAY","range":"RANGE_LAST_N","last_n":9007199254740993},"filters":[]}`)}}}
}
func publishInputFixture() pulsedefinition.PublishInput {
	return pulsedefinition.PublishInput{Environment: "target", Site: "target-site", SiteLUID: "target-site-id", Artifact: "artifacts/pulse-definition/example", DatasourceMap: []string{"ds-1=ds-2"}}
}

func TestPublishPlansWithoutMutationAndPreservesExactNumbers(t *testing.T) {
	d := &publishDependencies{bundle: publishBundleFixture()}
	input := publishInputFixture()
	input.Preview = true
	output, err := publish(context.Background(), d, input)
	if err != nil || d.writes != 0 || d.validations != 1 || !output.Complete || !strings.Contains(string(output.Plan.Metrics[0].Specification), "9007199254740993") || !strings.Contains(string(output.Plan.Metrics[0].Specification), `"id":"ds-2"`) {
		t.Fatalf("output=%#v err=%v writes=%d", output, err, d.writes)
	}
	if !strings.Contains(string(d.bundle.Metrics[0].Specification), `"id":"ds-1"`) {
		t.Fatal("source bundle was mutated")
	}
}

func TestPublishPrepareBundleAcceptsOwnExportAndPreservesMeaningfulSettings(t *testing.T) {
	bundle := publishBundleFixture()
	bundle.Configuration = json.RawMessage(`{"metadata":{"id":"definition-1","name":"Revenue","description":"Portable","business_context":{"owner":"finance"}},"specification":{"datasource":{"id":"ds-1"},"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Order Date"},"filters":[]},"is_running_total":false,"temporality":"TEMPORALITY_OVER_TIME","provider_extension":{"keep":"specification"}},"extension_options":{"allowed_dimensions":["Region"],"allowed_granularities":["GRANULARITY_BY_DAY"],"offset_from_today":2,"use_dynamic_offset":true,"provider_extension":{"keep":"extension"}},"representation_options":{"type":"NUMBER_FORMAT_TYPE_CURRENCY","sentiment_type":"SENTIMENT_TYPE_UP_IS_GOOD","currency_code":"CURRENCY_CODE_USD","provider_extension":{"keep":"representation"}},"insights_options":{"show_insights":true,"settings":[{"type":"INSIGHT_TYPE_TOP_DRIVERS","disabled":false}],"provider_extension":{"keep":"insights"}},"comparisons":{"comparisons":[{"compare_config":{"comparison":"TIME_COMPARISON_PREVIOUS_PERIOD"},"index":0}],"provider_extension":{"keep":"comparisons"}},"datasource_goals":[{"name":"Target","provider_extension":{"keep":"goal"}}],"related_links":[{"link_name":"Runbook","link_url":"https://example.test/runbook","provider_extension":{"keep":"link"}}],"certification":{"is_certified":true,"provider_extension":{"keep":"certification"}}}`)
	plan, err := preparePublish(publishInputFixture(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(plan.DefinitionConfiguration, &document); err != nil {
		t.Fatal(err)
	}
	if document["name"] != "Revenue" || document["description"] != "Portable" {
		t.Fatalf("identity=%#v", document)
	}
	specification := document["specification"].(map[string]any)
	if specification["provider_extension"].(map[string]any)["keep"] != "specification" || specification["datasource"].(map[string]any)["id"] != "ds-2" {
		t.Fatalf("specification=%#v", specification)
	}
	for section, marker := range map[string]string{"extension_options": "extension", "representation_options": "representation", "insights_options": "insights", "comparisons": "comparisons", "datasource_goals": "goal", "related_links": "link", "certification": "certification"} {
		value := document[section]
		encoded, err := json.Marshal(value)
		if err != nil || !strings.Contains(string(encoded), `"keep":"`+marker+`"`) {
			t.Fatalf("section %s=%s err=%v", section, encoded, err)
		}
	}
}

func TestPublishRetainsConfirmedIdentitiesOnPartialFailure(t *testing.T) {
	for _, stage := range []string{"create", "metric", "verify"} {
		t.Run(stage, func(t *testing.T) {
			d := &publishDependencies{bundle: publishBundleFixture(), failCreate: stage == "create", failMetric: stage == "metric", failVerify: stage == "verify"}
			output, err := publish(context.Background(), d, publishInputFixture())
			var structured *errs.Error
			if err == nil || output.Status != "partial" || output.Complete || len(output.Mappings) == 0 || output.Mappings[0].DestinationLUID != "new-definition" || !errors.As(err, &structured) {
				t.Fatalf("output=%#v err=%v", output, err)
			}
			if structured.Outcome != errs.OutcomeConfirmed {
				t.Fatalf("stage=%s recovery=%#v", stage, structured)
			}
		})
	}
}

func TestPublishRevalidatesBeforeWriting(t *testing.T) {
	d := &publishDependencies{bundle: publishBundleFixture(), failValidation: 2}
	output, err := publish(context.Background(), d, publishInputFixture())
	if err == nil || d.writes != 0 || d.validations != 2 || output.Complete {
		t.Fatalf("output=%#v err=%v writes=%d", output, err, d.writes)
	}
}

func TestPublishRejectsBadLocalMappingsAndUnknownSections(t *testing.T) {
	for _, mapping := range [][]string{nil, {"ds-1=ds-2", "ds-1=ds-3"}, {"other=ds-2"}, {"ds-1="}, {"ds-1=ds-2", "extra=ds-3"}} {
		input := publishInputFixture()
		input.DatasourceMap = mapping
		if _, err := preparePublish(input, publishBundleFixture()); err == nil {
			t.Fatalf("accepted mapping %v", mapping)
		}
	}
	bundle := publishBundleFixture()
	bundle.Configuration = json.RawMessage(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"ds-1"}},"unsupported_option":{"enabled":true}}`)
	if _, err := preparePublish(publishInputFixture(), bundle); err == nil || !strings.Contains(err.Error(), "losing configuration") {
		t.Fatalf("unknown configuration error=%v", err)
	}
	_, err := preparePublish(publishInputFixture(), bundle)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Phase != errs.PhaseValidation || structured.Outcome != errs.OutcomeNotAttempted {
		t.Fatalf("local rejection recovery=%#v", err)
	}
}

func TestPublishCompactMarksCappedDecisionDetailsIncomplete(t *testing.T) {
	output := pulsedefinition.PublishOutput{Plan: pulsedefinition.PublishPlan{ReviewComplete: true, Metrics: make([]pulsedefinition.PublishMetric, 11)}, Mappings: make([]pulsedefinition.PublishMapping, 21)}
	encoded, _ := json.Marshal(output.CompactOutput())
	if !strings.Contains(string(encoded), `"review_complete":false`) || !strings.Contains(string(encoded), `"details":"--full"`) {
		t.Fatalf("compact=%s", encoded)
	}
	if len(output.Plan.Metrics) != 11 || !output.Plan.ReviewComplete {
		t.Fatal("compact projection mutated full output")
	}
}
