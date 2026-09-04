package follow_test

import (
	"context"
	"testing"

	metricfollow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
)

type creator struct{ calls int }

func (c *creator) CreateSubscription(context.Context, metricfollow.CreateRequest) (metricfollow.CreateResult, error) {
	c.calls++
	return metricfollow.CreateResult{Status: "already_following"}, nil
}

func TestFollowPreviewsAndTreatsDuplicateAsConverged(t *testing.T) {
	c := &creator{}
	input := metricfollow.Input{MetricLUID: "metric-1", UserLUID: "user-1"}
	preview, err := metricfollow.New(c).Execute(context.Background(), input, true)
	if err != nil || preview.Result != nil || c.calls != 0 {
		t.Fatalf("preview=%#v calls=%d err=%v", preview, c.calls, err)
	}
	result, err := metricfollow.New(c).Execute(context.Background(), input, false)
	if err != nil || result.Result.Status != "already_following" {
		t.Fatalf("output=%#v err=%v", result, err)
	}
}

func TestFollowRequiresExactlyOneFollower(t *testing.T) {
	for _, input := range []metricfollow.Input{{MetricLUID: "metric-1"}, {MetricLUID: "metric-1", UserLUID: "u", GroupLUID: "g"}} {
		if _, err := metricfollow.New(&creator{}).Execute(context.Background(), input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}
