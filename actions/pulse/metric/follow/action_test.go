package follow_test

import (
	"context"
	"errors"
	"testing"

	metricfollow "github.com/ahillspace/tadx/actions/pulse/metric/follow"
	"github.com/ahillspace/tadx/internal/errs"
)

type creator struct{ calls int }

func (c *creator) CreateSubscription(context.Context, metricfollow.CreateRequest) (metricfollow.CreateResult, error) {
	c.calls++
	return metricfollow.CreateResult{Status: "already_following"}, nil
}

type resolver struct{ metrics, users, groups int }

func (r *resolver) ResolveMetric(_ context.Context, luid string) (metricfollow.Metric, error) {
	r.metrics++
	return metricfollow.Metric{LUID: luid}, nil
}
func (r *resolver) ResolveUser(_ context.Context, luid string) (metricfollow.User, error) {
	r.users++
	return metricfollow.User{LUID: luid}, nil
}
func (r *resolver) ResolveGroup(_ context.Context, luid string) (metricfollow.Group, error) {
	r.groups++
	return metricfollow.Group{LUID: luid}, nil
}

func TestFollowPreviewsAndTreatsDuplicateAsConverged(t *testing.T) {
	c := &creator{}
	r := &resolver{}
	input := metricfollow.Input{MetricLUID: "metric-1", UserLUID: "user-1"}
	preview, err := metricfollow.New(r, c).Execute(context.Background(), input, true)
	if err != nil || preview.Result != nil || c.calls != 0 || r.metrics != 1 || r.users != 1 {
		t.Fatalf("preview=%#v creator=%d resolver=%#v err=%v", preview, c.calls, r, err)
	}
	result, err := metricfollow.New(r, c).Execute(context.Background(), input, false)
	if err != nil || result.Result.Status != "already_following" {
		t.Fatalf("output=%#v err=%v", result, err)
	}
}

func TestFollowRequiresExactlyOneFollower(t *testing.T) {
	for _, input := range []metricfollow.Input{{MetricLUID: "metric-1"}, {MetricLUID: "metric-1", UserLUID: "u", GroupLUID: "g"}} {
		if _, err := metricfollow.New(&resolver{}, &creator{}).Execute(context.Background(), input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}

type missingUserResolver struct{ resolver }

func (missingUserResolver) ResolveUser(context.Context, string) (metricfollow.User, error) {
	return metricfollow.User{}, errors.New("user not found")
}

func TestFollowUserResolutionReportsRecoveryFacts(t *testing.T) {
	_, err := metricfollow.New(&missingUserResolver{}, &creator{}).Execute(context.Background(), metricfollow.Input{Environment: "production", Site: "marketing", MetricLUID: "metric-1", UserLUID: "user-1"}, true)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %v", err)
	}
	if structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeNotAttempted || structured.Prerequisite == nil || structured.Prerequisite.Kind != "user" || structured.Prerequisite.Resource != "user-1" {
		t.Fatalf("recovery facts = %#v", structured)
	}
}
