package metric

import (
	"context"
	"encoding/json"
	"testing"
)

type mutatingForkCreator struct{ *forkService }

func (s mutatingForkCreator) GetOrCreateMetric(ctx context.Context, request ForkCreateRequest) (ForkCreateResult, error) {
	result, err := s.forkService.GetOrCreateMetric(ctx, request)
	request.Specification["extension"].(map[string]any)["number"] = "changed"
	return result, err
}

func TestForkOwnsSourceAndSubmittedSpecification(t *testing.T) {
	s := &forkService{metric: Metric{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{
		"filters": []any{}, "extension": map[string]any{"number": json.Number("9007199254740993")},
	}}}
	output, err := fork(t.Context(), s, mutatingForkCreator{s}, s, ForkInput{MetricLUID: "metric-1", Timeframe: "LAST_30_DAYS"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for label, specification := range map[string]map[string]any{"source": s.metric.Specification, "plan": output.Plan.Specification} {
		if got := specification["extension"].(map[string]any)["number"]; got != json.Number("9007199254740993") {
			t.Fatalf("%s number = %v", label, got)
		}
	}
}
