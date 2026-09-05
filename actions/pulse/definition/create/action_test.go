package create_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	definitioncreate "github.com/ahillspace/tadx/actions/pulse/definition/create"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

func TestOutputGolden(t *testing.T) {
	request := definitioncreate.CreateRequest{
		Name: "Revenue", Description: "Recognized revenue",
		Specification:         definitioncreate.Specification{Datasource: definitioncreate.Datasource{ID: "datasource-1"}, BasicSpecification: definitioncreate.BasicSpecification{Measure: definitioncreate.Measure{Field: "Sales", Aggregation: "AGGREGATION_SUM"}, TimeDimension: definitioncreate.TimeDimension{Field: "Order Date"}, Filters: []definitioncreate.Filter{}}, Temporality: "TEMPORALITY_OVER_TIME"},
		ExtensionOptions:      definitioncreate.ExtensionOptions{AllowedDimensions: []string{"Region"}, AllowedGranularities: []string{"GRANULARITY_BY_MONTH"}},
		RepresentationOptions: definitioncreate.RepresentationOptions{Type: "NUMBER_FORMAT_TYPE_CURRENCY", SentimentType: "SENTIMENT_TYPE_UP_IS_GOOD", CurrencyCode: "CURRENCY_CODE_USD"},
		InsightsOptions:       definitioncreate.InsightsOptions{ShowInsights: true, Settings: []definitioncreate.InsightSetting{{Type: "INSIGHT_TYPE_TOP_DRIVERS"}}},
		Comparisons:           definitioncreate.Comparisons{Comparisons: []definitioncreate.Comparison{{CompareConfig: definitioncreate.CompareConfig{Comparison: "TIME_COMPARISON_PREVIOUS_PERIOD"}}}},
		DatasourceGoals:       []map[string]any{}, RelatedLinks: []map[string]any{},
	}
	output := definitioncreate.Output{
		Plan:   definitioncreate.Plan{Mode: "execute", Operation: "pulse.definition.create", Name: "Revenue", Datasource: "datasource-1", Measure: definitioncreate.Measure{Field: "Sales", Aggregation: "AGGREGATION_SUM"}, TimeField: "Order Date", Dimensions: []string{"Region"}, Fingerprint: "sha256:value", Request: request},
		Result: &definitioncreate.CreateResult{Status: "succeeded", DefinitionLUID: "definition-1", DefaultMetricLUID: "metric-1", DefaultMetricStatus: "ready", TableauRequestID: "request-1", PollRequestID: "poll-1"},
		Help:   []string{"tadx pulse metric inspect --id metric-1"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

type validator struct {
	input definitioncreate.FieldReferences
	calls int
	err   error
}

func (v *validator) ValidateDefinitionFields(_ context.Context, input definitioncreate.FieldReferences) error {
	v.calls++
	v.input = input
	return v.err
}

type finder struct {
	items []definitioncreate.ExistingDefinition
	calls int
}

func (f *finder) FindDefinitions(context.Context, string, string) ([]definitioncreate.ExistingDefinition, error) {
	f.calls++
	return append([]definitioncreate.ExistingDefinition(nil), f.items...), nil
}

type creator struct {
	request definitioncreate.CreateRequest
	result  definitioncreate.CreateResult
	calls   int
}

func (c *creator) CreateDefinition(_ context.Context, input definitioncreate.CreateRequest) (definitioncreate.CreateResult, error) {
	c.calls++
	c.request = input
	return c.result, nil
}

func TestCreatePlansSmallIntentAndAppliesOnlyWhenRequested(t *testing.T) {
	v, f := &validator{}, &finder{}
	c := &creator{result: definitioncreate.CreateResult{Status: "succeeded", DefinitionLUID: "definition-1", DefaultMetricLUID: "metric-1", TableauRequestID: "request-1"}}
	action := definitioncreate.New(v, f, c)
	input := definitioncreate.Input{Environment: "dev", Site: "sales", Intent: definitioncreate.Intent{
		Name: "Revenue", Description: "Recognized revenue.", DatasourceLUID: "datasource-1", MeasureField: "Sales", Aggregation: "SUM",
		TimeDimension: "Order Date", AllowedDimensions: []string{"Region", "Category", "Region"}, MinimumGranularity: "MONTH",
		NumberFormat: "CURRENCY", CurrencyCode: "USD", Sentiment: "UP", Temporality: "OVER_TIME", RunningTotal: true,
	}}
	preview, err := action.Execute(context.Background(), input, true)
	if err != nil || preview.Result != nil || c.calls != 0 || v.calls != 1 || f.calls != 1 {
		t.Fatalf("preview=%#v calls=%d/%d/%d err=%v", preview, v.calls, f.calls, c.calls, err)
	}
	if got := preview.Plan.Request.Specification.BasicSpecification.Measure.Aggregation; got != "AGGREGATION_SUM" {
		t.Fatalf("aggregation=%q", got)
	}
	if got := preview.Plan.Request.ExtensionOptions.AllowedDimensions; len(got) != 2 || got[0] != "Category" || got[1] != "Region" {
		t.Fatalf("dimensions=%#v", got)
	}
	result, err := action.Execute(context.Background(), input, false)
	if err != nil || result.Result == nil || result.Result.DefaultMetricLUID != "metric-1" || c.calls != 1 || v.calls != 3 || f.calls != 3 {
		t.Fatalf("result=%#v calls=%d/%d/%d err=%v", result, v.calls, f.calls, c.calls, err)
	}
	if c.request.Name != "Revenue" || c.request.Specification.Datasource.ID != "datasource-1" {
		t.Fatalf("request=%#v", c.request)
	}
}

func TestCreateSerializesZeroDimensionsAsAnEmptyArray(t *testing.T) {
	v, f, c := &validator{}, &finder{}, &creator{}
	plan, err := definitioncreate.New(v, f, c).Plan(context.Background(), definitioncreate.Input{Environment: "dev", Site: "sales", Intent: definitioncreate.Intent{
		Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Order Date",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Dimensions == nil || plan.Request.ExtensionOptions.AllowedDimensions == nil {
		t.Fatalf("plan dimensions=%#v request dimensions=%#v", plan.Dimensions, plan.Request.ExtensionOptions.AllowedDimensions)
	}
	compact := definitioncreate.Output{Plan: plan}.CompactOutput().(definitioncreate.CompactResult)
	if compact.Plan.Dimensions == nil {
		t.Fatalf("compact dimensions=%#v", compact.Plan.Dimensions)
	}
	encoded, err := json.Marshal(plan.Request)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	extension := payload["extension_options"].(map[string]any)
	if dimensions, ok := extension["allowed_dimensions"].([]any); !ok || len(dimensions) != 0 {
		t.Fatalf("payload dimensions=%#v", extension["allowed_dimensions"])
	}
}

func TestCreateRejectsUnsafeIntentBeforeRemoteCalls(t *testing.T) {
	tests := []definitioncreate.Intent{
		{},
		{Name: "Revenue", DatasourceLUID: "ds", MeasureField: "Sales", TimeDimension: "Date", Aggregation: "TOTAL"},
		{Name: "Snapshot", DatasourceLUID: "ds", MeasureField: "Inventory", TimeDimension: "Date", Aggregation: "SUM", Temporality: "LATEST", RunningTotal: true},
		{Name: "Revenue", DatasourceLUID: "ds", MeasureField: "Sales", TimeDimension: "Date", Aggregation: "SUM", NumberFormat: "CURRENCY", CurrencyCode: "NOT_A_CODE"},
	}
	for index, intent := range tests {
		v, f, c := &validator{}, &finder{}, &creator{}
		_, err := definitioncreate.New(v, f, c).Execute(context.Background(), definitioncreate.Input{Intent: intent}, false)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || v.calls != 0 || f.calls != 0 || c.calls != 0 {
			t.Fatalf("case %d error=%#v calls=%d/%d/%d", index, err, v.calls, f.calls, c.calls)
		}
	}
}

func TestCreatePreservesFieldValidationErrorContract(t *testing.T) {
	v := &validator{err: errors.New(`field "Calculation_margin" is already aggregated; use --aggregation USER`)}
	f, c := &finder{}, &creator{}
	_, err := definitioncreate.New(v, f, c).Execute(context.Background(), definitioncreate.Input{
		Environment: "dev",
		Site:        "sales",
		Intent: definitioncreate.Intent{
			Name:           "Margin",
			DatasourceLUID: "datasource-1",
			MeasureField:   "Calculation_margin",
			Aggregation:    "SUM",
			TimeDimension:  "Order Date",
		},
	}, false)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %#v", err)
	}
	if structured.ID != "pulse.definition.create.fields" || structured.Operation != "pulse.definition.create" || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("structured error = %#v", structured)
	}
	if v.calls != 1 || f.calls != 0 || c.calls != 0 {
		t.Fatalf("calls = validator:%d finder:%d creator:%d", v.calls, f.calls, c.calls)
	}
}

func TestCreateStopsOnExactNameDatasourceCollision(t *testing.T) {
	v := &validator{}
	f := &finder{items: []definitioncreate.ExistingDefinition{{LUID: "definition-old", Name: "Revenue", DatasourceLUID: "datasource-1"}}}
	c := &creator{}
	_, err := definitioncreate.New(v, f, c).Execute(context.Background(), definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Date", Aggregation: "SUM"}}, false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.definition.create.conflict" || c.calls != 0 {
		t.Fatalf("error=%#v calls=%d", err, c.calls)
	}
}

func TestApplyRejectsModifiedPlan(t *testing.T) {
	v, f, c := &validator{}, &finder{}, &creator{}
	action := definitioncreate.New(v, f, c)
	input := definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Date"}}
	plan, err := action.Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	plan.Request.Name = "Changed"
	_, err = action.Apply(context.Background(), input, plan)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || c.calls != 0 {
		t.Fatalf("error=%#v calls=%d", err, c.calls)
	}
}
