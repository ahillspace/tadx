package fork_test

import (
	"context"
	"testing"

	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
)

type service struct {
	created int
	metric  metricfork.Metric
}

func (s *service) GetMetric(context.Context, string) (metricfork.Metric, error) { return s.metric, nil }
func (s *service) GetDefinition(context.Context, string) (metricfork.Definition, error) {
	return metricfork.Definition{LUID: "definition-1", DatasourceLUID: "datasource-1", AllowedDimensions: []string{"Region"}}, nil
}
func (s *service) GetOrCreateMetric(_ context.Context, request metricfork.CreateRequest) (metricfork.CreateResult, error) {
	s.created++
	return metricfork.CreateResult{MetricLUID: "metric-fork", Created: true}, nil
}
func (s *service) ReconcileMetric(context.Context, metricfork.ExpectedMetric) (metricfork.Reconciliation, error) {
	return metricfork.Reconciliation{Status: "visible", Attempts: 1, OwnershipVerified: true, InventoryVisible: true}, nil
}

func TestForkPreservesUnknownFieldsAndPreviewsByDefault(t *testing.T) {
	s := &service{metric: metricfork.Metric{LUID: "metric-1", DefinitionLUID: "definition-1", SiteLUID: "site-1", Specification: map[string]any{"filters": []any{}, "comparison": map[string]any{"comparison": "PREVIOUS"}, "provider_extension": map[string]any{"keep": true}}}}
	input := metricfork.Input{Environment: "dev", Site: "sandbox", SiteLUID: "site-1", MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS", Filters: []metricfork.Filter{{Field: "Region", Values: []string{"West"}}}}
	preview, err := metricfork.New(s, s, s).Execute(context.Background(), input, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || s.created != 0 || preview.Plan.Specification["provider_extension"] == nil {
		t.Fatalf("preview=%#v created=%d", preview, s.created)
	}
	applied, err := metricfork.New(s, s, s).Execute(context.Background(), input, true)
	if err != nil || !applied.Applied || s.created != 1 || applied.Result == nil || applied.Result.ReconciliationStatus != "visible" {
		t.Fatalf("applied=%#v created=%d err=%v", applied, s.created, err)
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
