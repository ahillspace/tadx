package metric_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/errs"
)

type deleteBackend struct {
	targets     []pulsemetric.DeleteMetric
	calls       []string
	readError   error
	deleteError error
}

func (b *deleteBackend) GetMetric(_ context.Context, luid string) (pulsemetric.Metric, error) {
	b.calls = append(b.calls, "get:"+luid)
	if b.readError != nil {
		return pulsemetric.Metric{}, b.readError
	}
	if len(b.targets) == 0 {
		return pulsemetric.Metric{}, errors.New("missing target")
	}
	target := b.targets[0]
	b.targets = b.targets[1:]
	observed := pulsemetric.Metric{LUID: target.LUID, Name: target.Name, DefinitionLUID: target.DefinitionLUID, DefaultKnown: target.IsDefault != nil}
	if target.IsDefault != nil {
		observed.IsDefault = *target.IsDefault
	}
	return observed, nil
}
func (b *deleteBackend) DeleteMetric(_ context.Context, luid string) (pulsemetric.DeleteResult, error) {
	b.calls = append(b.calls, "delete:"+luid)
	return pulsemetric.DeleteResult{Status: "deleted", MetricLUID: luid, HTTPStatus: 204, TableauRequestID: "request-1"}, b.deleteError
}
func deleteInput() pulsemetric.DeleteInput {
	return pulsemetric.DeleteInput{Environment: "dev", Site: "sandbox", LUID: "metric-1"}
}
func deleteTarget() pulsemetric.DeleteMetric {
	return pulsemetric.DeleteMetric{LUID: "metric-1", Name: "Revenue"}
}

func TestDeleteRunsByDefaultWithExactRevalidation(t *testing.T) {
	b := &deleteBackend{targets: []pulsemetric.DeleteMetric{deleteTarget(), deleteTarget()}}
	output, err := delete(context.Background(), b, b, deleteInput())
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || output.Result.MetricLUID != "metric-1" {
		t.Fatalf("output=%#v", output)
	}
	want := []string{"get:metric-1", "get:metric-1", "delete:metric-1"}
	if !reflect.DeepEqual(b.calls, want) {
		t.Fatalf("calls=%v, want %v", b.calls, want)
	}
}
func TestDeletePreviewDoesNotDelete(t *testing.T) {
	b := &deleteBackend{targets: []pulsemetric.DeleteMetric{deleteTarget()}}
	in := deleteInput()
	in.Preview = true
	output, err := delete(context.Background(), b, b, in)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || !reflect.DeepEqual(b.calls, []string{"get:metric-1"}) {
		t.Fatalf("output=%#v calls=%v", output, b.calls)
	}
	if output.Plan.Mode != "preview" || len(output.Warnings) == 0 {
		t.Fatalf("preview=%#v", output)
	}
}

func TestDeleteRefusesKnownDefaultMetricWithoutWriting(t *testing.T) {
	defaultMetric := true
	for _, preview := range []bool{true, false} {
		t.Run(map[bool]string{true: "preview", false: "execute"}[preview], func(t *testing.T) {
			b := &deleteBackend{targets: []pulsemetric.DeleteMetric{{LUID: "metric-1", DefinitionLUID: "definition-1", IsDefault: &defaultMetric}}}
			in := deleteInput()
			in.Preview = preview
			output, err := delete(context.Background(), b, b, in)
			var structured *errs.Error
			if err == nil || !errors.As(err, &structured) || structured.Phase != errs.PhaseValidation || structured.Outcome != errs.OutcomeNotAttempted || !strings.Contains(err.Error(), "default") || output.Plan.Target.DefinitionLUID != "definition-1" {
				t.Fatalf("output=%#v err=%v", output, err)
			}
			if !reflect.DeepEqual(b.calls, []string{"get:metric-1"}) {
				t.Fatalf("calls=%v", b.calls)
			}
		})
	}
}

func TestDeleteRejectsWrongOrChangedIdentity(t *testing.T) {
	for _, targets := range [][]pulsemetric.DeleteMetric{
		{{LUID: "wrong"}},
		{deleteTarget(), {LUID: "wrong"}},
		{deleteTarget(), {}},
	} {
		b := &deleteBackend{targets: targets}
		_, err := delete(context.Background(), b, b, deleteInput())
		if err == nil {
			t.Fatal("expected exact identity failure")
		}
		for _, call := range b.calls {
			if call == "delete:metric-1" {
				t.Fatalf("deleted changed target: %v", b.calls)
			}
		}
	}
}
func TestDeleteRequiresExactSelector(t *testing.T) {
	for _, in := range []pulsemetric.DeleteInput{
		{Environment: "dev", Site: "sandbox", LUID: "  "},
	} {
		b := &deleteBackend{}
		_, err := delete(context.Background(), b, b, in)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || len(b.calls) != 0 {
			t.Fatalf("err=%v calls=%v", err, b.calls)
		}
	}
}
func TestDeletePreservesUpstreamFailureWithoutRetry(t *testing.T) {
	upstream := errors.New("upstream dependency rejection")
	b := &deleteBackend{targets: []pulsemetric.DeleteMetric{deleteTarget(), deleteTarget()}, deleteError: upstream}
	_, err := delete(context.Background(), b, b, deleteInput())
	if !errors.Is(err, upstream) || len(b.calls) != 3 {
		t.Fatalf("err=%v calls=%v", err, b.calls)
	}
}
func TestDeletePreservesMissingTarget(t *testing.T) {
	upstream := errors.New("upstream target not found")
	b := &deleteBackend{readError: upstream}
	_, err := delete(context.Background(), b, b, deleteInput())
	if !errors.Is(err, upstream) || len(b.calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, b.calls)
	}
}
