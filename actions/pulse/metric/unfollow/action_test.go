package unfollow_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	metricunfollow "github.com/ahillspace/tadx/actions/pulse/metric/unfollow"
	render "github.com/ahillspace/tadx/internal/output"
)

type service struct {
	subscriptions []metricunfollow.Subscription
	listed        int
	deleted       []string
}

func (s *service) ListSubscriptions(context.Context, string) ([]metricunfollow.Subscription, error) {
	s.listed++
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
	if len(result.Help) != 1 || result.Help[0] != "tadx pulse metric followers --id metric-1" {
		t.Fatalf("help=%#v", result.Help)
	}
	var rendered bytes.Buffer
	if err := render.RenderWithOptions(&rendered, result, render.Options{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "help[1]: tadx pulse metric followers --id metric-1") {
		t.Fatalf("rendered output omitted relationship help: %s", rendered.String())
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
	if len(output.Help) != 0 {
		t.Fatalf("help=%#v", output.Help)
	}
	var rendered bytes.Buffer
	if err := render.RenderWithOptions(&rendered, output, render.Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.String(), "followers --id") {
		t.Fatalf("rendered output contains malformed followers help: %s", rendered.String())
	}
	if strings.Contains(rendered.String(), "help") {
		t.Fatalf("rendered output contains unavailable help: %s", rendered.String())
	}
}

func TestUnfollowRejectsMixedAndIncompleteSelectorsBeforeRemoteCalls(t *testing.T) {
	tests := []metricunfollow.Input{
		{},
		{SubscriptionLUID: "sub-1", MetricLUID: "metric-1"},
		{SubscriptionLUID: "sub-1", UserLUID: "user-1"},
		{SubscriptionLUID: "sub-1", GroupLUID: "group-1"},
		{MetricLUID: "metric-1"},
		{UserLUID: "user-1"},
		{GroupLUID: "group-1"},
		{MetricLUID: "metric-1", UserLUID: "user-1", GroupLUID: "group-1"},
	}
	for _, input := range tests {
		s := &service{}
		if _, err := metricunfollow.New(s, s).Execute(context.Background(), input, false); err == nil {
			t.Fatalf("invalid selectors accepted: %#v", input)
		}
		if s.listed != 0 || len(s.deleted) != 0 {
			t.Fatalf("invalid selectors caused remote calls: input=%#v listed=%d deleted=%#v", input, s.listed, s.deleted)
		}
	}
}
