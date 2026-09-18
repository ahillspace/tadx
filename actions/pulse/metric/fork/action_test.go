package fork_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type service struct {
	created int
	metric  metricfork.Metric
	request metricfork.CreateRequest
}

type failingReconciler struct{ *service }

func (f failingReconciler) ReconcileMetric(context.Context, metricfork.ExpectedMetric) (metricfork.Reconciliation, error) {
	return metricfork.Reconciliation{}, errors.New("readback unavailable")
}

func (s *service) ResolveFilterFields(_ context.Context, _ string, fields []string) ([]string, error) {
	out := append([]string(nil), fields...)
	for i, field := range out {
		if field == "Region caption" {
			out[i] = "Region"
		}
	}
	return out, nil
}

func TestForkResolvesCaptionBeforeReplacingInheritedFilter(t *testing.T) {
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{map[string]any{"field": "Region", "categorical_values": []any{map[string]any{"string_value": "East"}}}}}}}
	in := metricfork.Input{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []metricfork.Filter{{Field: "Region caption", Values: []string{"West"}}}}
	out, err := metricfork.New(s, s, s).Execute(context.Background(), in, true)
	if err != nil || len(out.Plan.Filters) != 1 || out.Plan.Filters[0].Field != "Region" || s.created != 0 {
		t.Fatalf("preview=%#v err=%v writes=%d", out, err, s.created)
	}
	_, err = metricfork.New(s, s, s).Execute(context.Background(), in, false)
	if err != nil || s.created != 1 {
		t.Fatalf("err=%v writes=%d", err, s.created)
	}
	filters := s.request.Specification["filters"].([]any)
	if len(filters) != 1 || filters[0].(map[string]any)["field"] != "Region" {
		t.Fatalf("filters=%#v", filters)
	}
}

func TestForkMergesAliasAndRawFiltersAndRejectsConflictingOperators(t *testing.T) {
	for _, exclude := range []bool{false, true} {
		s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}}}
		in := metricfork.Input{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []metricfork.Filter{{Field: "Region caption", Values: []string{"West"}}, {Field: "Region", Values: []string{"East", "West"}, Exclude: exclude}}}
		out, err := metricfork.New(s, s, s).Execute(context.Background(), in, true)
		if exclude {
			if err == nil || !strings.Contains(err.Error(), "conflicting") || s.created != 0 {
				t.Fatalf("expected conflicting alias/raw filters: out=%#v err=%v", out, err)
			}
		} else if err != nil || len(out.Plan.Filters) != 1 || len(out.Plan.Filters[0].Values) != 2 || out.Plan.Filters[0].Values[0] != "East" || out.Plan.Filters[0].Values[1] != "West" {
			t.Fatalf("out=%#v err=%v", out, err)
		}
	}
}

type driftingAliasService struct {
	service
	resolutions int
}

func (s *driftingAliasService) GetDefinition(ctx context.Context, id string) (metricfork.Definition, error) {
	d, err := s.service.GetDefinition(ctx, id)
	d.AllowedDimensions = append(d.AllowedDimensions, "OtherRegion")
	return d, err
}
func (s *driftingAliasService) ResolveFilterFields(context.Context, string, []string) ([]string, error) {
	s.resolutions++
	if s.resolutions > 1 {
		return []string{"OtherRegion"}, nil
	}
	return []string{"Region"}, nil
}
func TestForkRejectsAliasRetargetBeforeWrite(t *testing.T) {
	s := &driftingAliasService{service: service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}}}}
	_, err := metricfork.New(s, s, s).Execute(context.Background(), metricfork.Input{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []metricfork.Filter{{Field: "Region caption", Values: []string{"West"}}}}, false)
	if err == nil || !strings.Contains(err.Error(), "changed") || s.created != 0 {
		t.Fatalf("err=%v writes=%d", err, s.created)
	}
}

func TestForkBoundsCombinedAliasAndRawFilterValues(t *testing.T) {
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}}}
	left, right := []string{}, []string{}
	for i := 0; i < 10001; i++ {
		if i < 5000 {
			left = append(left, strconv.Itoa(i))
		} else {
			right = append(right, strconv.Itoa(i))
		}
	}
	_, err := metricfork.New(s, s, s).Execute(context.Background(), metricfork.Input{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []metricfork.Filter{{Field: "Region caption", Values: left}, {Field: "Region", Values: right}}}, false)
	if err == nil || !strings.Contains(err.Error(), "combined values") || s.created != 0 {
		t.Fatalf("err=%v writes=%d", err, s.created)
	}
}

func (s *service) GetMetric(context.Context, string) (metricfork.Metric, error) { return s.metric, nil }
func (s *service) GetDefinition(context.Context, string) (metricfork.Definition, error) {
	return metricfork.Definition{LUID: "definition-1", DatasourceLUID: "datasource-1", AllowedDimensions: []string{"Region"}, AllowedGranularities: []string{"GRANULARITY_BY_DAY", "GRANULARITY_BY_WEEK", "GRANULARITY_BY_MONTH", "GRANULARITY_BY_QUARTER", "GRANULARITY_BY_YEAR"}}, nil
}
func (s *service) GetOrCreateMetric(_ context.Context, request metricfork.CreateRequest) (metricfork.CreateResult, error) {
	s.created++
	s.request = request
	return metricfork.CreateResult{MetricLUID: "metric-fork", Created: true}, nil
}
func (s *service) ReconcileMetric(_ context.Context, expected metricfork.ExpectedMetric) (metricfork.Reconciliation, error) {
	return metricfork.Reconciliation{Status: "verified", Attempts: 1, OwnershipVerified: true, SpecificationVerified: true, SavedSpecification: expected.Specification, SavedDefinition: metricfork.SavedDefinition{LUID: expected.DefinitionLUID, DatasourceLUID: expected.DatasourceLUID}}, nil
}

func TestForkPreservesUnknownFieldsAndPreviewsByDefault(t *testing.T) {
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}, "comparison": map[string]any{"comparison": "PREVIOUS"}, "provider_extension": map[string]any{"keep": true}}}}
	input := metricfork.Input{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []metricfork.Filter{{Field: "Region", Values: []string{"West"}}}}
	preview, err := metricfork.New(s, s, s).Execute(context.Background(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || s.created != 0 || preview.Plan.Specification["provider_extension"] == nil {
		t.Fatalf("preview=%#v created=%d", preview, s.created)
	}
	result, err := metricfork.New(s, s, s).Execute(context.Background(), input, false)
	if err != nil || s.created != 1 || result.Result == nil || result.Result.ReconciliationStatus != "verified" {
		t.Fatalf("result=%#v created=%d err=%v", result, s.created, err)
	}
}

func TestForkPreservesCreatedMetricWhenReconciliationFails(t *testing.T) {
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}
	input := metricfork.Input{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}
	output, err := metricfork.New(s, s, failingReconciler{s}).Execute(context.Background(), input, false)
	var structured *errs.Error
	if err == nil || output.Result == nil || output.Result.MetricLUID != "metric-fork" || !errors.As(err, &structured) || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeConfirmed {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if len(output.Help) != 1 || !strings.Contains(output.Help[0], "pulse metric inspect") || !strings.Contains(output.Help[0], "--full") || strings.Contains(output.Help[0], "--preview") {
		t.Fatalf("stale recovery help=%#v", output.Help)
	}
}

type uncertainCreateService struct{ service }

func (s *uncertainCreateService) GetOrCreateMetric(context.Context, metricfork.CreateRequest) (metricfork.CreateResult, error) {
	return metricfork.CreateResult{MetricLUID: "metric-uncertain", Created: true}, errors.New("provider response was interrupted")
}

func TestForkRetainsIdentityWhenCreateOutcomeIsUncertain(t *testing.T) {
	s := &uncertainCreateService{service: service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}}
	output, err := metricfork.New(s, s, s).Execute(context.Background(), metricfork.Input{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}, false)
	var structured *errs.Error
	if err == nil || output.Result == nil || output.Result.MetricLUID != "metric-uncertain" || !errors.As(err, &structured) || structured.Outcome != errs.OutcomeUnknown {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if len(output.Help) != 1 || !strings.Contains(output.Help[0], "metric inspect") || !strings.Contains(output.Help[0], "metric-uncertain") || !strings.Contains(output.Help[0], "--full") {
		t.Fatalf("recovery help=%#v", output.Help)
	}
}

type existingForkService struct{ service }

func (s *existingForkService) GetOrCreateMetric(_ context.Context, request metricfork.CreateRequest) (metricfork.CreateResult, error) {
	s.created++
	s.request = request
	return metricfork.CreateResult{MetricLUID: "metric-existing", Created: false}, nil
}
func (*existingForkService) ReconcileMetric(context.Context, metricfork.ExpectedMetric) (metricfork.Reconciliation, error) {
	return metricfork.Reconciliation{}, errors.New("readback unavailable")
}

func TestForkPreservesExistingMetricWhenReconciliationFails(t *testing.T) {
	s := &existingForkService{service: service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}}
	output, err := metricfork.New(s, s, s).Execute(context.Background(), metricfork.Input{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}, false)
	if err == nil || output.Result == nil || output.Result.MetricLUID != "metric-existing" || output.Result.Status != "existing" || output.Result.Created {
		t.Fatalf("output=%#v err=%v", output, err)
	}
}

func TestForkPreservesIntegerSpecFidelity(t *testing.T) {
	const bigInt = int64(9007199254740993) // 2^53 + 1, not representable as float64
	spec := map[string]any{
		"filters": []any{},
		"comparison": map[string]any{
			"offset": json.Number("9007199254740993"),
			"period": json.Number("30"),
		},
	}
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: spec}}
	input := metricfork.Input{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}
	result, err := metricfork.New(s, s, s).Execute(context.Background(), input, false)
	if err != nil || result.Result == nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	comparison, ok := s.request.Specification["comparison"].(map[string]any)
	if !ok {
		t.Fatalf("comparison missing: %#v", s.request.Specification)
	}
	offset, ok := comparison["offset"].(json.Number)
	if !ok {
		t.Fatalf("offset is %T not json.Number: %#v", comparison["offset"], comparison["offset"])
	}
	got, err := offset.Int64()
	if err != nil || got != bigInt {
		t.Fatalf("offset=%v (%v) want %d", got, err, bigInt)
	}
	// The rendered request must serialize the integer exactly, not in float form.
	data, err := json.Marshal(s.request.Specification)
	if err != nil {
		t.Fatal(err)
	}
	if want := "9007199254740993"; !strings.Contains(string(data), want) {
		t.Fatalf("serialized spec %s missing exact integer %s", data, want)
	}
}

func TestForkAcceptsOnlySupportedCustomDayWindows(t *testing.T) {
	for _, days := range []int{7, 14, 30, 60, 90} {
		t.Run(strconv.Itoa(days), func(t *testing.T) {
			s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}
			input := metricfork.Input{MetricLUID: "metric-1", Timeframe: "CUSTOM_N_DAYS", CustomDays: days, CustomDaysSet: true}
			preview, err := metricfork.New(s, s, s).Execute(context.Background(), input, true)
			if err != nil {
				t.Fatal(err)
			}
			period := preview.Plan.Specification["measurement_period"].(map[string]any)["last_x_period"].(map[string]any)["period"]
			if period != days || s.created != 0 {
				t.Fatalf("preview period=%v writes=%d", period, s.created)
			}
			if _, err := metricfork.New(s, s, s).Execute(context.Background(), input, false); err != nil || s.created != 1 {
				t.Fatalf("execute writes=%d err=%v", s.created, err)
			}
		})
	}
}

func TestForkOutputGolden(t *testing.T) {
	output := metricfork.Output{
		Plan: metricfork.Plan{
			Mode: "execute", Operation: "pulse.metric.fork", Environment: "dev", Site: "sandbox",
			SourceMetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1",
			Timeframe: "LAST_30_DAYS",
			Filters:   []metricfork.Filter{{Field: "Region", Values: []string{"West"}}},
			Specification: map[string]any{
				"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG", "last_x_period": map[string]any{"period": 30, "period_type": "GRANULARITY_BY_DAY", "include_current_period": true}},
				"filters":            []any{map[string]any{"field": "Region", "operator": "OPERATOR_EQUAL", "categorical_values": []any{map[string]any{"string_value": "West"}}, "include_null": false}},
			},
			DefinitionFilters: []any{}, DefinitionFiltersKnown: true,
			Fingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		Result: &metricfork.Result{
			Status: "created", MetricLUID: "metric-fork", MetricName: "Revenue West", Created: true,
			ReconciliationStatus: "verified", ReconciliationAttempts: 1, OwnershipVerified: true, SpecificationVerified: true,
			SavedSpecification: map[string]any{"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG"}},
			SavedDefinition:    metricfork.SavedDefinition{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1"},
			RequestID:          "request-1", ReconciliationRequestID: "request-2",
		},
		Help: []string{"Saved metric configuration and definition linkage verified; current values and generated insights are not read by TADX."},
	}
	output.Result.SavedSpecification = output.Plan.Specification
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

func TestForkRejectsNoChangeAndDisallowedDimension(t *testing.T) {
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}, "measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_CURRENT_PARTIAL"}}}}
	for _, input := range []metricfork.Input{{MetricLUID: "metric-1"}, {MetricLUID: "metric-1", Filters: []metricfork.Filter{{Field: "Secret", Values: []string{"x"}}}}} {
		_, err := metricfork.New(s, s, s).Execute(context.Background(), input, false)
		if err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
		if len(input.Filters) > 0 && (!strings.Contains(err.Error(), `field "Secret"`) || !strings.Contains(err.Error(), "allowed")) {
			t.Fatalf("disallowed dimension error=%v", err)
		}
	}
}
