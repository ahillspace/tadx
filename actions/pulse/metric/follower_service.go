package metric

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

type FollowerTarget struct {
	Environment, Site string
	Reader            CachedFollowerReader
}

type CachedFollowerReader interface {
	FollowersReader
	Source() *readsource.Metadata
}

type LiveFollowerPort interface {
	FollowersReader
	FollowResolver
	FollowCreator
	UnfollowReader
	UnfollowDeleter
	Publish(context.Context, string) error
}

type FollowerSession struct {
	Environment, Site string
	Port              LiveFollowerPort
}

type FollowerProvider interface {
	CacheFollowerTarget(string) (FollowerTarget, error)
	OpenFollowers(context.Context, string, string, string, bool) (FollowerSession, error)
	CheckPrincipalCapability(bool) error
	CacheSetupError(string, error) error
	Now() time.Time
}

func (s *Service) ListPulseMetricFollowers(ctx context.Context, input FollowersInput) (FollowersOutput, error) {
	if err := followersValidateInput(input); err != nil {
		return FollowersOutput{}, err
	}
	if s == nil || s.ports.Followers == nil {
		return FollowersOutput{}, &errs.Error{ID: "pulse.metric.followers.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.metric.followers", Summary: "Pulse metric followers are not configured.", Retryable: errs.Bool(false)}
	}
	if input.Cache {
		target, err := s.ports.Followers.CacheFollowerTarget(input.Environment)
		if err != nil {
			return FollowersOutput{}, s.ports.Followers.CacheSetupError(input.Environment, err)
		}
		input.Environment, input.Site = target.Environment, target.Site
		reader := target.Reader
		output, err := runFollowers(ctx, reader, input)
		if err == nil {
			output.Source = reader.Source()
			output.RequestID = ""
		}
		return output, err
	}
	session, err := s.ports.Followers.OpenFollowers(ctx, input.Environment, input.Site, "pulse.metric.followers", false)
	if err != nil {
		return FollowersOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := runFollowers(ctx, session.Port, input)
	if err != nil {
		return output, err
	}
	output.Source = new(readsource.Live(s.ports.Followers.Now().UTC()))
	if err := session.Port.Publish(ctx, input.MetricLUID); err != nil {
		output.Warnings = append(output.Warnings, "The live follower snapshot could not be cached; the previous cached snapshot was preserved.")
	}
	return output, nil
}

func (s *Service) FollowPulseMetric(ctx context.Context, input FollowInput, preview bool) (FollowOutput, error) {
	if err := followValidateInput(input); err != nil {
		return FollowOutput{}, err
	}
	if s == nil || s.ports.Followers == nil {
		return FollowOutput{}, &errs.Error{ID: "pulse.metric.follow.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.metric.follow", Summary: "Pulse metric follow is not configured.", Retryable: errs.Bool(false)}
	}
	if err := s.ports.Followers.CheckPrincipalCapability(input.GroupLUID != ""); err != nil {
		return FollowOutput{}, err
	}
	session, err := s.ports.Followers.OpenFollowers(ctx, input.Environment, input.Site, "pulse.metric.follow", true)
	if err != nil {
		return FollowOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return runFollow(ctx, session.Port, session.Port, input, preview)
}

func (s *Service) UnfollowPulseMetric(ctx context.Context, input UnfollowInput, preview bool) (UnfollowOutput, error) {
	if err := unfollowValidateInput(input); err != nil {
		return UnfollowOutput{}, err
	}
	if s == nil || s.ports.Followers == nil {
		return UnfollowOutput{}, &errs.Error{ID: "pulse.metric.unfollow.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.metric.unfollow", Summary: "Pulse metric unfollow is not configured.", Retryable: errs.Bool(false)}
	}
	session, err := s.ports.Followers.OpenFollowers(ctx, input.Environment, input.Site, "pulse.metric.unfollow", true)
	if err != nil {
		return UnfollowOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return runUnfollow(ctx, session.Port, session.Port, input, preview)
}
