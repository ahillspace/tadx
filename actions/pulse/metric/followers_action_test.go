package metric_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/errs"
)

type followersReader struct {
	metric        pulsemetric.Metric
	metricErr     error
	subscriptions []pulsemetric.Subscription
	getCalls      int
	listCalls     int
}

type followersLiveError struct{}

func (followersLiveError) Error() string            { return "HTTP 404" }
func (followersLiveError) Retryable() bool          { return false }
func (followersLiveError) CorrectiveAction() string { return "Verify the Pulse metric LUID." }
func (followersLiveError) RequestID() string        { return "request-404" }

func (r *followersReader) GetMetric(context.Context, string) (pulsemetric.Metric, error) {
	r.getCalls++
	return r.metric, r.metricErr
}

func (r *followersReader) ListSubscriptions(context.Context, string) ([]pulsemetric.Subscription, error) {
	r.listCalls++
	return r.subscriptions, nil
}

func TestFollowersReturnsExactRelationships(t *testing.T) {
	reader := &followersReader{
		metric:        pulsemetric.Metric{LUID: "metric-1"},
		subscriptions: []pulsemetric.Subscription{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"}},
	}
	output, err := pulsemetric.Followers(context.Background(), reader, pulsemetric.FollowersInput{MetricLUID: "metric-1"})
	if err != nil || output.Count != 1 || output.Subscriptions[0].LUID != "sub-1" {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	if reader.getCalls != 1 || reader.listCalls != 1 {
		t.Fatalf("get calls = %d, list calls = %d", reader.getCalls, reader.listCalls)
	}
}

func TestFollowersNormalizesSuccessfulEmptySubscriptions(t *testing.T) {
	reader := &followersReader{metric: pulsemetric.Metric{LUID: "metric-1"}}
	output, err := pulsemetric.Followers(context.Background(), reader, pulsemetric.FollowersInput{MetricLUID: "metric-1"})
	if err != nil {
		t.Fatal(err)
	}
	if output.Count != 0 || output.Subscriptions == nil {
		t.Fatalf("output=%#v", output)
	}
	full, ok := output.FullOutput().(pulsemetric.FollowersOutput)
	if !ok || full.Subscriptions == nil {
		t.Fatalf("full output=%#v", output.FullOutput())
	}
}

func TestFollowersRejectsMalformedMetricSelectorBeforeReading(t *testing.T) {
	reader := &followersReader{}
	_, err := pulsemetric.Followers(context.Background(), reader, pulsemetric.FollowersInput{MetricLUID: "metric with spaces"})
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
	reader := &followersReader{metricErr: followersLiveError{}}

	_, err := pulsemetric.Followers(context.Background(), reader, pulsemetric.FollowersInput{
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
