package metric

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type followCreator struct{ calls int }

func (c *followCreator) CreateSubscription(context.Context, FollowCreateRequest) (FollowCreateResult, error) {
	c.calls++
	return FollowCreateResult{Status: "already_following"}, nil
}

type followResolver struct{ metrics, users, groups int }

func (r *followResolver) ResolveMetric(_ context.Context, luid string) (FollowMetric, error) {
	r.metrics++
	return FollowMetric{LUID: luid}, nil
}
func (r *followResolver) ResolveUser(_ context.Context, luid string) (FollowUser, error) {
	r.users++
	return FollowUser{LUID: luid}, nil
}
func (r *followResolver) ResolveGroup(_ context.Context, luid string) (FollowGroup, error) {
	r.groups++
	return FollowGroup{LUID: luid}, nil
}

func TestFollowPreviewsAndTreatsDuplicateAsConverged(t *testing.T) {
	c := &followCreator{}
	r := &followResolver{}
	input := FollowInput{MetricLUID: "metric-1", UserLUID: "user-1"}
	preview, err := follow(context.Background(), r, c, input, true)
	if err != nil || preview.Result != nil || c.calls != 0 || r.metrics != 1 || r.users != 1 {
		t.Fatalf("preview=%#v creator=%d resolver=%#v err=%v", preview, c.calls, r, err)
	}
	result, err := follow(context.Background(), r, c, input, false)
	if err != nil || result.Result.Status != "already_following" {
		t.Fatalf("output=%#v err=%v", result, err)
	}
}

func TestFollowRequiresExactlyOneFollower(t *testing.T) {
	for _, input := range []FollowInput{{MetricLUID: "metric-1"}, {MetricLUID: "metric-1", UserLUID: "u", GroupLUID: "g"}} {
		if _, err := follow(context.Background(), &followResolver{}, &followCreator{}, input, false); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
}

type followMissingUserResolver struct{ followResolver }

func (followMissingUserResolver) ResolveUser(context.Context, string) (FollowUser, error) {
	return FollowUser{}, errors.New("user not found")
}

func TestFollowUserResolutionReportsRecoveryFacts(t *testing.T) {
	_, err := follow(context.Background(), &followMissingUserResolver{}, &followCreator{}, FollowInput{Environment: "production", Site: "marketing", MetricLUID: "metric-1", UserLUID: "user-1"}, true)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %v", err)
	}
	if structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeNotAttempted || structured.Prerequisite == nil || structured.Prerequisite.Kind != "user" || structured.Prerequisite.Resource != "user-1" {
		t.Fatalf("recovery facts = %#v", structured)
	}
}
