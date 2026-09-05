package fork_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	render "github.com/ahillspace/tadx/internal/output"
)

type service struct {
	created int
	metric  metricfork.Metric
	request metricfork.CreateRequest
}

func (s *service) GetMetric(context.Context, string) (metricfork.Metric, error) { return s.metric, nil }
func (s *service) GetDefinition(context.Context, string) (metricfork.Definition, error) {
	return metricfork.Definition{LUID: "definition-1", DatasourceLUID: "datasource-1", AllowedDimensions: []string{"Region"}}, nil
}
func (s *service) GetOrCreateMetric(_ context.Context, request metricfork.CreateRequest) (metricfork.CreateResult, error) {
	s.created++
	s.request = request
	return metricfork.CreateResult{MetricLUID: "metric-fork", Created: true}, nil
}
func (s *service) ReconcileMetric(context.Context, metricfork.ExpectedMetric) (metricfork.Reconciliation, error) {
	return metricfork.Reconciliation{Status: "visible", Attempts: 1, OwnershipVerified: true, InventoryVisible: true}, nil
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
	if err != nil || s.created != 1 || result.Result == nil || result.Result.ReconciliationStatus != "visible" {
		t.Fatalf("result=%#v created=%d err=%v", result, s.created, err)
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

func TestForkOutputGolden(t *testing.T) {
	output := metricfork.Output{
		Plan: metricfork.Plan{
			Mode: "execute", Operation: "pulse.metric.fork", Environment: "dev", Site: "sandbox",
			SourceMetricLUID: "metric-1", DefinitionLUID: "definition-1", DatasourceLUID: "datasource-1",
			Timeframe: "LAST_30_DAYS",
			Filters:   []metricfork.Filter{{Field: "Region", Values: []string{"West"}}},
			Specification: map[string]any{
				"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG"},
			},
			Fingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		Result: &metricfork.Result{
			Status: "created", MetricLUID: "metric-fork", MetricName: "Revenue West", Created: true,
			ReconciliationStatus: "visible", ReconciliationAttempts: 1, OwnershipVerified: true, InventoryVisible: true,
			RequestID: "request-1", ReconciliationRequestID: "request-2",
		},
		Help: []string{"tadx pulse metric inspect --id metric-fork"},
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

func TestForkRejectsNoChangeAndDisallowedDimension(t *testing.T) {
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"filters": []any{}}}}
	for _, input := range []metricfork.Input{{MetricLUID: "metric-1"}, {MetricLUID: "metric-1", Filters: []metricfork.Filter{{Field: "Secret", Values: []string{"x"}}}}} {
		if _, err := metricfork.New(s, s, s).Execute(context.Background(), input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}
