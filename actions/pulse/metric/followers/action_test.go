package followers_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/pulse/metric/followers"
	"github.com/ahillspace/tadx/internal/errs"
)

type reader struct {
	metric        followers.Metric
	metricErr     error
	subscriptions []followers.Subscription
	getCalls      int
	listCalls     int
}

type liveError struct{}

func (liveError) Error() string            { return "HTTP 404" }
func (liveError) Retryable() bool          { return false }
func (liveError) CorrectiveAction() string { return "Verify the Pulse metric LUID." }
func (liveError) RequestID() string        { return "request-404" }

func (r *reader) GetMetric(context.Context, string) (followers.Metric, error) {
	r.getCalls++
	return r.metric, r.metricErr
}

func (r *reader) ListSubscriptions(context.Context, string) ([]followers.Subscription, error) {
	r.listCalls++
	return r.subscriptions, nil
}

func TestFollowersReturnsExactRelationships(t *testing.T) {
	reader := &reader{
		metric:        followers.Metric{LUID: "metric-1"},
		subscriptions: []followers.Subscription{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"}},
	}
	output, err := followers.New(reader).Execute(context.Background(), followers.Input{MetricLUID: "metric-1"})
	if err != nil || output.Count != 1 || output.Subscriptions[0].LUID != "sub-1" {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if reader.getCalls != 1 || reader.listCalls != 1 {
		t.Fatalf("get calls = %d, list calls = %d", reader.getCalls, reader.listCalls)
	}
}

func TestFollowersNormalizesSuccessfulEmptySubscriptions(t *testing.T) {
	reader := &reader{metric: followers.Metric{LUID: "metric-1"}}
	output, err := followers.New(reader).Execute(context.Background(), followers.Input{MetricLUID: "metric-1"})
	if err != nil {
		t.Fatal(err)
	}
	if output.Count != 0 || output.Subscriptions == nil {
		t.Fatalf("output=%#v", output)
	}
	full, ok := output.FullOutput().(followers.Output)
	if !ok || full.Subscriptions == nil {
		t.Fatalf("full output=%#v", output.FullOutput())
	}
}

func TestFollowersRejectsMalformedMetricSelectorBeforeReading(t *testing.T) {
	reader := &reader{}
	_, err := followers.New(reader).Execute(context.Background(), followers.Input{MetricLUID: "metric with spaces"})
	if err == nil {
		t.Fatal("expected malformed metric selector error")
	}
	if reader.getCalls != 0 || reader.listCalls != 0 {
		t.Fatalf("reader calls = get:%d list:%d", reader.getCalls, reader.listCalls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "pulse.metric.followers.usage" || !strings.Contains(err.Error(), "metric with spaces") {
		t.Fatalf("error = %#v", err)
	}
}

func TestFollowersRejectsMissingMetricBeforeListingSubscriptions(t *testing.T) {
	reader := &reader{metricErr: liveError{}}

	_, err := followers.New(reader).Execute(context.Background(), followers.Input{
		Environment: "dev",
		Site:        "test-site",
		MetricLUID:  "missing-metric",
	})
	if err == nil {
		t.Fatal("expected missing metric error")
	}
	if reader.getCalls != 1 || reader.listCalls != 0 {
		t.Fatalf("get calls = %d, list calls = %d", reader.getCalls, reader.listCalls)
	}
	var got *errs.Error
	if !errors.As(err, &got) {
		t.Fatalf("error = %T %v", err, err)
	}
	if got.ID != "pulse.metric.followers.failed" || got.TableauRequestID != "request-404" || got.Retryable == nil || *got.Retryable {
		t.Fatalf("structured error = %#v", got)
	}
	if got.CorrectiveAction != "Verify the Pulse metric LUID." {
		t.Fatalf("corrective action = %q", got.CorrectiveAction)
	}
}
