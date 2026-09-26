package create_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		Plan:   definitioncreate.Plan{Mode: "execute", Operation: "pulse.definition.create", Environment: "dev", Site: "sales", Name: "Revenue", Datasource: "datasource-1", Measure: definitioncreate.Measure{Field: "Sales", Aggregation: "AGGREGATION_SUM"}, TimeField: "Order Date", Dimensions: []string{"Region"}, Fingerprint: "sha256:value", Request: request},
		Result: &definitioncreate.CreateResult{Status: "succeeded", DefinitionLUID: "definition-1", DefaultMetricLUID: "metric-1", DefaultMetricStatus: "ready", TableauRequestID: "request-1", PollRequestID: "poll-1"},
		Help:   []string{"tadx pulse metric inspect --id metric-1 --environment dev"},
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
	resolved *definitioncreate.FieldReferences
	input    definitioncreate.FieldReferences
	calls    int
	err      error
}

func (v *validator) ResolveDefinitionFields(ctx context.Context, input definitioncreate.FieldReferences) (definitioncreate.FieldReferences, error) {
	err := v.ValidateDefinitionFields(ctx, input)
	if v.resolved != nil {
		return *v.resolved, err
	}
	return input, err
}

func TestCreateCanonicalizesFieldReferencesBeforePreviewAndWrite(t *testing.T) {
	refs := definitioncreate.FieldReferences{DatasourceLUID: "datasource-1", MeasureField: "[Calculation_1]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[date_raw]", AllowedDimensions: []string{"[region_raw]"}}
	v, f, c := &validator{resolved: &refs}, &finder{}, &creator{result: definitioncreate.CreateResult{DefinitionLUID: "definition-1", DefaultMetricLUID: "metric-1"}}
	input := definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Revenue", TimeDimension: "Order Date", AllowedDimensions: []string{"Region"}}}
	a := definitioncreate.New(v, f, c)
	out, err := a.Execute(context.Background(), input, true)
	if err != nil || out.Plan.Measure.Field != refs.MeasureField || out.Plan.TimeField != refs.TimeDimension || len(out.Plan.Dimensions) != 1 || out.Plan.Dimensions[0] != refs.AllowedDimensions[0] || c.calls != 0 {
		t.Fatalf("preview=%#v err=%v writes=%d", out, err, c.calls)
	}
	_, err = a.Execute(context.Background(), input, false)
	if err != nil || c.calls != 1 || c.request.Specification.BasicSpecification.Measure.Field != refs.MeasureField || c.request.Specification.BasicSpecification.TimeDimension.Field != refs.TimeDimension || c.request.ExtensionOptions.AllowedDimensions[0] != refs.AllowedDimensions[0] {
		t.Fatalf("request=%#v err=%v writes=%d", c.request, err, c.calls)
	}
}

func TestCreatePlanOwnsNormalizedInput(t *testing.T) {
	dimensions := []string{"Region"}
	resolved := definitioncreate.FieldReferences{DatasourceLUID: "datasource-1", MeasureField: "Sales", Aggregation: "AGGREGATION_SUM", TimeDimension: "Order Date", AllowedDimensions: []string{"[Region]"}}
	input := definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Order Date", AllowedDimensions: dimensions}}
	plan, err := definitioncreate.New(&validator{resolved: &resolved}, &finder{}, &creator{}).Plan(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	dimensions[0] = "Changed"
	resolved.AllowedDimensions[0] = "Changed"
	if got := plan.Request.ExtensionOptions.AllowedDimensions; len(got) != 1 || got[0] != "[Region]" {
		t.Fatalf("plan retained caller-owned dimensions: %v", got)
	}
	if got := plan.Dimensions; len(got) != 1 || got[0] != "[Region]" {
		t.Fatalf("projection retained caller-owned dimensions: %v", got)
	}
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
	if got := preview.Plan.Request.ExtensionOptions.AllowedDimensions; len(got) != 2 || got[0] != "Region" || got[1] != "Category" {
		t.Fatalf("dimensions=%#v", got)
	}
	result, err := action.Execute(context.Background(), input, false)
	if err != nil || result.Result == nil || result.Result.DefaultMetricLUID != "metric-1" || c.calls != 1 || v.calls != 2 || f.calls != 2 {
		t.Fatalf("result=%#v calls=%d/%d/%d err=%v", result, v.calls, f.calls, c.calls, err)
	}
	if c.request.Name != "Revenue" || c.request.Specification.Datasource.ID != "datasource-1" {
		t.Fatalf("request=%#v", c.request)
	}
}

func TestCreatePreviewWarnsWhenCompactPlanNeedsFullReview(t *testing.T) {
	dimensions := make([]string, 51)
	for index := range dimensions {
		dimensions[index] = "Dimension_" + string(rune('A'+index%26)) + string(rune('0'+index/26))
	}
	v, f, c := &validator{}, &finder{}, &creator{}
	input := definitioncreate.Input{Environment: "dev", Site: "sales", Intent: definitioncreate.Intent{
		Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Order Date", Aggregation: "SUM", AllowedDimensions: dimensions,
	}}
	output, err := definitioncreate.New(v, f, c).Execute(context.Background(), input, true)
	if err != nil || !output.CompactOutput().(definitioncreate.CompactResult).Plan.RequiresFull {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if len(output.Help) != 1 || !strings.Contains(output.Help[0], "--full") || strings.Contains(output.Help[0], "without --preview") {
		t.Fatalf("incomplete review help=%#v", output.Help)
	}
}

func TestCreateRejectsZeroDimensionsBeforeRemoteCalls(t *testing.T) {
	v, f, c := &validator{}, &finder{}, &creator{}
	_, err := definitioncreate.New(v, f, c).Plan(context.Background(), definitioncreate.Input{Environment: "dev", Site: "sales", Intent: definitioncreate.Intent{
		Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Order Date",
	}})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || v.calls != 0 || f.calls != 0 || c.calls != 0 {
		t.Fatalf("error=%#v calls=%d/%d/%d", err, v.calls, f.calls, c.calls)
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
			Name:              "Margin",
			DatasourceLUID:    "datasource-1",
			MeasureField:      "Calculation_margin",
			Aggregation:       "SUM",
			TimeDimension:     "Order Date",
			AllowedDimensions: []string{"Region"},
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
	_, err := definitioncreate.New(v, f, c).Execute(context.Background(), definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Date", Aggregation: "SUM", AllowedDimensions: []string{"Region"}}}, false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.definition.create.conflict" || c.calls != 0 {
		t.Fatalf("error=%#v calls=%d", err, c.calls)
	}
}

func TestApplyRejectsModifiedPlan(t *testing.T) {
	v, f, c := &validator{}, &finder{}, &creator{}
	action := definitioncreate.New(v, f, c)
	input := definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Date", AllowedDimensions: []string{"Region"}}}
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

func TestRetainedApplyStillDetectsFieldAndCollisionDrift(t *testing.T) {
	for _, collision := range []bool{false, true} {
		v, f, c := &validator{}, &finder{}, &creator{}
		action := definitioncreate.New(v, f, c)
		input := definitioncreate.Input{Intent: definitioncreate.Intent{Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", TimeDimension: "Date", AllowedDimensions: []string{"Region"}}}
		plan, err := action.Plan(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if collision {
			f.items = []definitioncreate.ExistingDefinition{{LUID: "new-definition", Name: "Revenue", DatasourceLUID: "datasource-1"}}
		} else {
			v.err = errors.New("selected field is now excluded")
		}
		_, err = action.Apply(context.Background(), input, plan)
		if err == nil || c.calls != 0 || v.calls != 2 {
			t.Fatalf("collision=%v err=%v fields=%d creates=%d", collision, err, v.calls, c.calls)
		}
	}
}
