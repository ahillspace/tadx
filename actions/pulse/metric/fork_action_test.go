package metric

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

	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type forkService struct {
	created int
	metric  Metric
	request ForkCreateRequest
}

type forkFailingReconciler struct{ *forkService }

func (f forkFailingReconciler) ReconcileMetric(context.Context, ForkExpectedMetric) (ForkReconciliation, error) {
	return ForkReconciliation{}, errors.New("readback unavailable")
}

func (s *forkService) ResolveFilterFields(_ context.Context, _ string, fields []string) ([]string, error) {
	out := append([]string(nil), fields...)
	for i, field := range out {
		if field == "Region caption" {
			out[i] = "Region"
		}
	}
	return out, nil
}

func TestForkResolvesCaptionBeforeReplacingInheritedFilter(t *testing.T) {
	s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{map[string]any{"field": "Region", "categorical_values": []any{map[string]any{"string_value": "East"}}}}}}}
	in := ForkInput{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []ForkFilter{{Field: "Region caption", Values: []string{"West"}}}}
	out, err := fork(context.Background(), s, s, s, in, true)
	if err != nil || len(out.Plan.Filters) != 1 || out.Plan.Filters[0].Field != "Region" || s.created != 0 {
		t.Fatalf("preview=%#v err=%v writes=%d", out, err, s.created)
	}
	_, err = fork(context.Background(), s, s, s, in, false)
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
		s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}}}
		in := ForkInput{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []ForkFilter{{Field: "Region caption", Values: []string{"West"}}, {Field: "Region", Values: []string{"East", "West"}, Exclude: exclude}}}
		out, err := fork(context.Background(), s, s, s, in, true)
		if exclude {
			if err == nil || !strings.Contains(err.Error(), "conflicting") || s.created != 0 {
				t.Fatalf("expected conflicting alias/raw filters: out=%#v err=%v", out, err)
			}
		} else if err != nil || len(out.Plan.Filters) != 1 || len(out.Plan.Filters[0].Values) != 2 || out.Plan.Filters[0].Values[0] != "East" || out.Plan.Filters[0].Values[1] != "West" {
			t.Fatalf("out=%#v err=%v", out, err)
		}
	}
}

type forkDriftingAliasService struct {
	forkService
	resolutions int
}

func (s *forkDriftingAliasService) GetDefinition(ctx context.Context, id string) (ForkDefinition, error) {
	d, err := s.forkService.GetDefinition(ctx, id)
	d.AllowedDimensions = append(d.AllowedDimensions, "OtherRegion")
	return d, err
}
func (s *forkDriftingAliasService) ResolveFilterFields(context.Context, string, []string) ([]string, error) {
	s.resolutions++
	if s.resolutions > 1 {
		return []string{"OtherRegion"}, nil
	}
	return []string{"Region"}, nil
}
func TestForkRejectsAliasRetargetBeforeWrite(t *testing.T) {
	s := &forkDriftingAliasService{forkService: forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}}}}
	_, err := fork(context.Background(), s, s, s, ForkInput{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []ForkFilter{{Field: "Region caption", Values: []string{"West"}}}}, false)
	if err == nil || !strings.Contains(err.Error(), "changed") || s.created != 0 {
		t.Fatalf("err=%v writes=%d", err, s.created)
	}
}

func TestForkBoundsCombinedAliasAndRawFilterValues(t *testing.T) {
	s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}}}
	left, right := []string{}, []string{}
	for i := 0; i < 10001; i++ {
		if i < 5000 {
			left = append(left, strconv.Itoa(i))
		} else {
			right = append(right, strconv.Itoa(i))
		}
	}
	_, err := fork(context.Background(), s, s, s, ForkInput{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []ForkFilter{{Field: "Region caption", Values: left}, {Field: "Region", Values: right}}}, false)
	if err == nil || !strings.Contains(err.Error(), "combined values") || s.created != 0 {
		t.Fatalf("err=%v writes=%d", err, s.created)
	}
}

func (s *forkService) GetMetric(context.Context, string) (Metric, error) {
	return s.metric, nil
}
func (s *forkService) GetDefinition(context.Context, string) (ForkDefinition, error) {
	return ForkDefinition{LUID: "definition-1", DatasourceLUID: "datasource-1", AllowedDimensions: []string{"Region"}, AllowedGranularities: []string{"GRANULARITY_BY_DAY", "GRANULARITY_BY_WEEK", "GRANULARITY_BY_MONTH", "GRANULARITY_BY_QUARTER", "GRANULARITY_BY_YEAR"}}, nil
}
func (s *forkService) GetOrCreateMetric(_ context.Context, request ForkCreateRequest) (ForkCreateResult, error) {
	s.created++
	s.request = request
	return ForkCreateResult{MetricLUID: "metric-fork", Created: true}, nil
}
func (s *forkService) ReconcileMetric(_ context.Context, expected ForkExpectedMetric) (ForkReconciliation, error) {
	return ForkReconciliation{Status: "verified", Attempts: 1, OwnershipVerified: true, SpecificationVerified: true, SavedSpecification: expected.Specification, SavedDefinition: ForkSavedDefinition{LUID: expected.DefinitionLUID, DatasourceLUID: expected.DatasourceLUID}}, nil
}

func TestForkPreservesUnknownFieldsAndPreviewsByDefault(t *testing.T) {
	s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}, "comparison": map[string]any{"comparison": "PREVIOUS"}, "provider_extension": map[string]any{"keep": true}}}}
	input := ForkInput{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []ForkFilter{{Field: "Region", Values: []string{"West"}}}}
	preview, err := fork(context.Background(), s, s, s, input, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || s.created != 0 || preview.Plan.Specification["provider_extension"] == nil {
		t.Fatalf("preview=%#v created=%d", preview, s.created)
	}
	result, err := fork(context.Background(), s, s, s, input, false)
	if err != nil || s.created != 1 || result.Result == nil || result.Result.ReconciliationStatus != "verified" {
		t.Fatalf("result=%#v created=%d err=%v", result, s.created, err)
	}
}

func TestForkPreservesCreatedMetricWhenReconciliationFails(t *testing.T) {
	s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}
	input := ForkInput{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}
	output, err := fork(context.Background(), s, s, forkFailingReconciler{s}, input, false)
	var structured *errs.Error
	if err == nil || output.Result == nil || output.Result.MetricLUID != "metric-fork" || !errors.As(err, &structured) || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeConfirmed {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if len(output.Help) != 1 || !strings.Contains(output.Help[0], "pulse metric inspect") || !strings.Contains(output.Help[0], "--full") || strings.Contains(output.Help[0], "--preview") {
		t.Fatalf("stale recovery help=%#v", output.Help)
	}
}

type forkUncertainCreateService struct{ forkService }

func (s *forkUncertainCreateService) GetOrCreateMetric(context.Context, ForkCreateRequest) (ForkCreateResult, error) {
	return ForkCreateResult{MetricLUID: "metric-uncertain", Created: true}, errors.New("provider response was interrupted")
}

func TestForkRetainsIdentityWhenCreateOutcomeIsUncertain(t *testing.T) {
	s := &forkUncertainCreateService{forkService: forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}}
	output, err := fork(context.Background(), s, s, s, ForkInput{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}, false)
	var structured *errs.Error
	if err == nil || output.Result == nil || output.Result.MetricLUID != "metric-uncertain" || !errors.As(err, &structured) || structured.Outcome != errs.OutcomeUnknown {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if len(output.Help) != 1 || !strings.Contains(output.Help[0], "metric inspect") || !strings.Contains(output.Help[0], "metric-uncertain") || !strings.Contains(output.Help[0], "--full") {
		t.Fatalf("recovery help=%#v", output.Help)
	}
}

type forkExistingForkService struct{ forkService }

func (s *forkExistingForkService) GetOrCreateMetric(_ context.Context, request ForkCreateRequest) (ForkCreateResult, error) {
	s.created++
	s.request = request
	return ForkCreateResult{MetricLUID: "metric-existing", Created: false}, nil
}
func (*forkExistingForkService) ReconcileMetric(context.Context, ForkExpectedMetric) (ForkReconciliation, error) {
	return ForkReconciliation{}, errors.New("readback unavailable")
}

func TestForkPreservesExistingMetricWhenReconciliationFails(t *testing.T) {
	s := &forkExistingForkService{forkService: forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}}
	output, err := fork(context.Background(), s, s, s, ForkInput{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}, false)
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
	s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: spec}}
	input := ForkInput{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}
	result, err := fork(context.Background(), s, s, s, input, false)
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
			s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}}}}
			input := ForkInput{MetricLUID: "metric-1", Timeframe: "CUSTOM_N_DAYS", CustomDays: days, CustomDaysSet: true}
			preview, err := fork(context.Background(), s, s, s, input, true)
			if err != nil {
				t.Fatal(err)
			}
			period := preview.Plan.Specification["measurement_period"].(map[string]any)["last_x_period"].(map[string]any)["period"]
			if period != days || s.created != 0 {
				t.Fatalf("preview period=%v writes=%d", period, s.created)
			}
			if _, err := fork(context.Background(), s, s, s, input, false); err != nil || s.created != 1 {
				t.Fatalf("execute writes=%d err=%v", s.created, err)
			}
		})
	}
}

func TestForkOutputGolden(t *testing.T) {
	output := ForkOutput{
		Plan: ForkPlan{
			Mode: "execute", Operation: "pulse.metric.fork", Environment: "dev", Site: "sandbox",
			SourceMetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1",
			Timeframe: "LAST_30_DAYS",
			Filters:   []ForkFilter{{Field: "Region", Values: []string{"West"}}},
			Specification: map[string]any{
				"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG", "last_x_period": map[string]any{"period": 30, "period_type": "GRANULARITY_BY_DAY", "include_current_period": true}},
				"filters":            []any{map[string]any{"field": "Region", "operator": "OPERATOR_EQUAL", "categorical_values": []any{map[string]any{"string_value": "West"}}, "include_null": false}},
			},
			DefinitionFilters: []any{}, DefinitionFiltersKnown: true,
			Fingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		Result: &ForkResult{
			Status: "created", MetricLUID: "metric-fork", MetricName: "Revenue West", Created: true,
			ReconciliationStatus: "verified", ReconciliationAttempts: 1, OwnershipVerified: true, SpecificationVerified: true,
			SavedSpecification: map[string]any{"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG"}},
			SavedDefinition:    ForkSavedDefinition{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1"},
			RequestID:          "request-1", ReconciliationRequestID: "request-2",
		},
		Help: []string{"Saved metric configuration and definition linkage verified; current values and generated insights are not read by TADX."},
	}
	output.Result.SavedSpecification = output.Plan.Specification
	forkAssertGolden(t, "compact.toon", output, false)
	forkAssertGolden(t, "full.toon", output, true)
}

func forkAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "fork", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func TestForkRejectsNoChangeAndDisallowedDimension(t *testing.T) {
	s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}, "measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_CURRENT_PARTIAL"}}}}
	for _, input := range []ForkInput{{MetricLUID: "metric-1"}, {MetricLUID: "metric-1", Filters: []ForkFilter{{Field: "Secret", Values: []string{"x"}}}}} {
		_, err := fork(context.Background(), s, s, s, input, false)
		if err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
		if len(input.Filters) > 0 && (!strings.Contains(err.Error(), `field "Secret"`) || !strings.Contains(err.Error(), "allowed")) {
			t.Fatalf("disallowed dimension error=%v", err)
		}
	}
}
