package get_test

import (
	"context"
	"testing"

	metricget "github.com/ahillspace/tadx/actions/pulse/metric/get"
)

type reader struct{ metric metricget.Metric }

func (r reader) GetMetric(context.Context, string) (metricget.Metric, error) { return r.metric, nil }

func TestGetRequiresAndVerifiesExactMetric(t *testing.T) {
	metric := metricget.Metric{LUID: "metric-1", Name: "Revenue", DefinitionLUID: "definition-1", Specification: map[string]any{"provider_extension": map[string]any{"keep": true}}}
	output, err := metricget.New(reader{metric}).Execute(context.Background(), metricget.Input{Environment: "dev", Site: "sandbox", LUID: "metric-1"})
	if err != nil || output.Metric.Specification["provider_extension"] == nil {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if _, err := metricget.New(reader{metric}).Execute(context.Background(), metricget.Input{LUID: "other"}); err == nil {
		t.Fatal("mismatched metric accepted")
	}
}
