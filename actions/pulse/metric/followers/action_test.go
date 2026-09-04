package followers_test

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/actions/pulse/metric/followers"
)

type reader struct{}

func (reader) ListSubscriptions(context.Context, string) ([]followers.Subscription, error) {
	return []followers.Subscription{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"}}, nil
}

func TestFollowersReturnsExactRelationships(t *testing.T) {
	output, err := followers.New(reader{}).Execute(context.Background(), followers.Input{MetricLUID: "metric-1"})
	if err != nil || output.Count != 1 || output.Subscriptions[0].LUID != "sub-1" {
		t.Fatalf("output=%#v err=%v", output, err)
	}
}
