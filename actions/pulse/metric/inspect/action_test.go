package inspect_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metricget "github.com/ahillspace/tadx/actions/pulse/metric/inspect"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

type reader struct{ metric metricget.Metric }

func (r reader) GetMetric(context.Context, string) (metricget.Metric, error) { return r.metric, nil }

type missingMetricReader struct{}

func (missingMetricReader) GetMetric(context.Context, string) (metricget.Metric, error) {
	return metricget.Metric{}, missingMetricError{}
}

type missingMetricError struct{}

func (missingMetricError) Error() string          { return "provider says metric is missing" }
func (missingMetricError) HTTPStatus() int        { return http.StatusNotFound }
func (missingMetricError) TableauCode() string    { return "not-found" }
func (missingMetricError) TableauSummary() string { return "missing metric" }
func (missingMetricError) TableauDetail() string  { return "metric is not visible" }

func TestInspectRequiresAndVerifiesExactMetric(t *testing.T) {
	metric := metricget.Metric{LUID: "metric-1", Name: "Revenue", DefinitionLUID: "definition-1", Specification: map[string]any{"provider_extension": map[string]any{"keep": true}}}
	output, err := metricget.New(reader{metric}).Execute(context.Background(), metricget.Input{Environment: "dev", Site: "sandbox", LUID: "metric-1"})
	if err != nil || output.Metric.Specification["provider_extension"] == nil {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if _, err := metricget.New(reader{metric}).Execute(context.Background(), metricget.Input{LUID: "other"}); err == nil {
		t.Fatal("mismatched metric accepted")
	}
}

func TestInspectUsesResourceSpecificRecoveryForMissingMetric(t *testing.T) {
	_, err := metricget.New(missingMetricReader{}).Execute(context.Background(), metricget.Input{Environment: "production", Site: "marketing", LUID: "metric-1"})
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "pulse.metric.inspect.not_found" || structured.Resource != "metric-1" || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeNotAttempted {
		t.Fatalf("error=%#v", err)
	}
	if !strings.Contains(structured.Summary, "metric-1") || !strings.Contains(structured.CorrectiveAction, "metric-1") || strings.Contains(strings.ToLower(structured.CorrectiveAction), "server configuration") {
		t.Fatalf("resource recovery=%#v", structured)
	}
	if envelope := errs.Structure(err); envelope.Error.UpstreamStatus != http.StatusNotFound {
		t.Fatalf("upstream status=%#v", envelope)
	}
}

func TestInspectOutputGolden(t *testing.T) {
	source := readsource.Live(time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC))
	output := metricget.Output{
		Status: "found", Environment: "dev", Site: "sandbox",
		Metric: metricget.Metric{
			LUID: "metric-1", Name: "Revenue", DefinitionLUID: "definition-1", SiteLUID: "site-1", IsDefault: true,
			Specification: map[string]any{"measurement_period": map[string]any{"granularity": "GRANULARITY_BY_DAY"}},
		},
		RequestID: "request-1",
		Source:    &source,
		Help:      []string{"tadx pulse metric list --definition definition-1"},
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
