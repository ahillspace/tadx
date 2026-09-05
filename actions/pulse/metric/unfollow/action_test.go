package unfollow_test

import (
	"context"
	"testing"

	metricunfollow "github.com/ahillspace/tadx/actions/pulse/metric/unfollow"
)

type service struct {
	subscriptions []metricunfollow.Subscription
	deleted       []string
}

func (s *service) ListSubscriptions(context.Context, string) ([]metricunfollow.Subscription, error) {
	return s.subscriptions, nil
}
func (s *service) DeleteSubscription(_ context.Context, luid string) error {
	s.deleted = append(s.deleted, luid)
	return nil
}

func TestUnfollowResolvesOneExactRelationshipAndRevalidates(t *testing.T) {
	s := &service{subscriptions: []metricunfollow.Subscription{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "GROUP", FollowerLUID: "group-1"}}}
	input := metricunfollow.Input{MetricLUID: "metric-1", GroupLUID: "group-1"}
	preview, err := metricunfollow.New(s, s).Execute(context.Background(), input, true)
	if err != nil || preview.Plan.SubscriptionLUID != "sub-1" || len(s.deleted) != 0 {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	result, err := metricunfollow.New(s, s).Execute(context.Background(), input, false)
	if err != nil || result.Result == nil || len(s.deleted) != 1 || s.deleted[0] != "sub-1" {
		t.Fatalf("output=%#v deleted=%#v err=%v", result, s.deleted, err)
	}
}

func TestUnfollowRejectsAmbiguousRelationship(t *testing.T) {
	s := &service{subscriptions: []metricunfollow.Subscription{{LUID: "sub-1", FollowerType: "USER", FollowerLUID: "user-1"}, {LUID: "sub-2", FollowerType: "USER", FollowerLUID: "user-1"}}}
	if _, err := metricunfollow.New(s, s).Execute(context.Background(), metricunfollow.Input{MetricLUID: "metric-1", UserLUID: "user-1"}, false); err == nil {
		t.Fatal("ambiguous relationship accepted")
	}
}

func TestUnfollowAcceptsExactSubscriptionWithoutResolution(t *testing.T) {
	s := &service{}
	output, err := metricunfollow.New(s, s).Execute(context.Background(), metricunfollow.Input{SubscriptionLUID: "sub-1"}, false)
	if err != nil || output.Result == nil || len(s.deleted) != 1 || s.deleted[0] != "sub-1" {
		t.Fatalf("output=%#v deleted=%#v err=%v", output, s.deleted, err)
	}
}
