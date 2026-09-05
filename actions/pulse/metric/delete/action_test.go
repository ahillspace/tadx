package delete_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	action "github.com/ahillspace/tadx/actions/pulse/metric/delete"
	"github.com/ahillspace/tadx/internal/errs"
)

type backend struct {
	targets     []action.Metric
	calls       []string
	readError   error
	deleteError error
}

func (b *backend) GetMetric(_ context.Context, luid string) (action.Metric, error) {
	b.calls = append(b.calls, "get:"+luid)
	if b.readError != nil {
		return action.Metric{}, b.readError
	}
	if len(b.targets) == 0 {
		return action.Metric{}, errors.New("missing target")
	}
	target := b.targets[0]
	b.targets = b.targets[1:]
	return target, nil
}
func (b *backend) DeleteMetric(_ context.Context, luid string) (action.Result, error) {
	b.calls = append(b.calls, "delete:"+luid)
	return action.Result{Status: "deleted", MetricLUID: luid, HTTPStatus: 204, TableauRequestID: "request-1"}, b.deleteError
}
func input() action.Input   { return action.Input{Environment: "dev", Site: "sandbox", LUID: "metric-1"} }
func target() action.Metric { return action.Metric{LUID: "metric-1", Name: "Revenue"} }

func TestDeleteRunsByDefaultWithExactRevalidation(t *testing.T) {
	b := &backend{targets: []action.Metric{target(), target()}}
	output, err := action.New(b, b).Execute(context.Background(), input())
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
func TestPreviewDoesNotDelete(t *testing.T) {
	b := &backend{targets: []action.Metric{target()}}
	in := input()
	in.Preview = true
	output, err := action.New(b, b).Execute(context.Background(), in)
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
func TestDeleteRejectsWrongOrChangedIdentity(t *testing.T) {
	for _, targets := range [][]action.Metric{
		{{LUID: "wrong"}},
		{target(), {LUID: "wrong"}},
		{target(), {}},
	} {
		b := &backend{targets: targets}
		_, err := action.New(b, b).Execute(context.Background(), input())
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
func TestDeleteRequiresExplicitTarget(t *testing.T) {
	for _, in := range []action.Input{
		{Site: "sandbox", LUID: "metric-1"},
		{Environment: "dev", LUID: "metric-1"},
		{Environment: "dev", Site: "sandbox", LUID: "  "},
	} {
		b := &backend{}
		_, err := action.New(b, b).Execute(context.Background(), in)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || len(b.calls) != 0 {
			t.Fatalf("err=%v calls=%v", err, b.calls)
		}
	}
}
func TestDeletePreservesUpstreamFailureWithoutRetry(t *testing.T) {
	upstream := errors.New("upstream dependency rejection")
	b := &backend{targets: []action.Metric{target(), target()}, deleteError: upstream}
	_, err := action.New(b, b).Execute(context.Background(), input())
	if !errors.Is(err, upstream) || len(b.calls) != 3 {
		t.Fatalf("err=%v calls=%v", err, b.calls)
	}
}
func TestDeletePreservesMissingTarget(t *testing.T) {
	upstream := errors.New("upstream target not found")
	b := &backend{readError: upstream}
	_, err := action.New(b, b).Execute(context.Background(), input())
	if !errors.Is(err, upstream) || len(b.calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, b.calls)
	}
}
