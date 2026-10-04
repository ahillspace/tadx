package metric

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/readsource"
)

// These helpers follow composition's input-validation boundary before invoking operations.
func list(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	return New(Ports{Read: workflowMetricProvider{target: ReadTarget{Environment: input.Environment, Site: input.Site}, port: workflowMetricPort{ListReader: reader}}}).ListPulseMetrics(ctx, input)
}

func inspect(ctx context.Context, reader InspectReader, input InspectInput) (InspectOutput, error) {
	return New(Ports{Read: workflowMetricProvider{target: ReadTarget{Environment: input.Environment, Site: input.Site}, port: workflowMetricPort{InspectReader: reader}}}).InspectPulseMetric(ctx, input)
}

type workflowMetricPort struct {
	ListReader
	InspectReader
}

func (workflowMetricPort) Source() *readsource.Metadata { return nil }
func (workflowMetricPort) Publish()                     {}

type workflowMetricProvider struct {
	target ReadTarget
	port   workflowMetricPort
}

func (p workflowMetricProvider) CacheTarget(string) (ReadTarget, error) {
	return p.target, nil
}
func (p workflowMetricProvider) CachedList(ReadTarget) CachedListPort {
	return p.port
}
func (p workflowMetricProvider) CachedInspect(ReadTarget) CachedInspectPort {
	return p.port
}
func (p workflowMetricProvider) Open(context.Context, string, string, string) (ReadSession, error) {
	return ReadSession{ReadTarget: p.target, List: p.port, Inspect: p.port}, nil
}
func (workflowMetricProvider) CacheSetupError(_, _ string, err error) error { return err }
func (workflowMetricProvider) Now() time.Time                               { return time.Now() }

func deleteWorkflow(ctx context.Context, reader DeleteReader, deleter Deleter, input DeleteInput) (DeleteOutput, error) {
	if err := deleteValidateInput(input); err != nil {
		return DeleteOutput{}, err
	}
	return runDelete(ctx, reader, deleter, input)
}

func fork(ctx context.Context, reader ForkReader, creator ForkCreator, reconciler ForkReconciler, input ForkInput, preview bool) (ForkOutput, error) {
	if err := forkValidateInput(&input); err != nil {
		return ForkOutput{}, err
	}
	return runFork(ctx, reader, creator, reconciler, input, preview)
}

func followers(ctx context.Context, reader FollowersReader, input FollowersInput) (FollowersOutput, error) {
	if err := followersValidateInput(input); err != nil {
		return FollowersOutput{}, err
	}
	return runFollowers(ctx, reader, input)
}

func follow(ctx context.Context, resolver FollowResolver, creator FollowCreator, input FollowInput, preview bool) (FollowOutput, error) {
	if err := followValidateInput(input); err != nil {
		return FollowOutput{}, err
	}
	return runFollow(ctx, resolver, creator, input, preview)
}

func unfollow(ctx context.Context, reader UnfollowReader, deleter UnfollowDeleter, input UnfollowInput, preview bool) (UnfollowOutput, error) {
	if err := unfollowValidateInput(input); err != nil {
		return UnfollowOutput{}, err
	}
	return runUnfollow(ctx, reader, deleter, input, preview)
}
