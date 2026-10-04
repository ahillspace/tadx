package metric_test

import (
	"context"
	"time"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/readsource"
)

// These helpers follow composition's input-validation boundary before invoking operations.
func list(ctx context.Context, reader pulsemetric.ListReader, input pulsemetric.ListInput) (pulsemetric.ListOutput, error) {
	return pulsemetric.New(workflowMetricProvider{target: pulsemetric.ReadTarget{Environment: input.Environment, Site: input.Site}, port: workflowMetricPort{ListReader: reader}}).ListPulseMetrics(ctx, input)
}

func inspect(ctx context.Context, reader pulsemetric.InspectReader, input pulsemetric.InspectInput) (pulsemetric.InspectOutput, error) {
	return pulsemetric.New(workflowMetricProvider{target: pulsemetric.ReadTarget{Environment: input.Environment, Site: input.Site}, port: workflowMetricPort{InspectReader: reader}}).InspectPulseMetric(ctx, input)
}

type workflowMetricPort struct {
	pulsemetric.ListReader
	pulsemetric.InspectReader
}

func (workflowMetricPort) Source() *readsource.Metadata { return nil }
func (workflowMetricPort) Publish()                     {}

type workflowMetricProvider struct {
	target pulsemetric.ReadTarget
	port   workflowMetricPort
}

func (p workflowMetricProvider) CacheTarget(string) (pulsemetric.ReadTarget, error) {
	return p.target, nil
}
func (p workflowMetricProvider) CachedList(pulsemetric.ReadTarget) pulsemetric.CachedListPort {
	return p.port
}
func (p workflowMetricProvider) CachedInspect(pulsemetric.ReadTarget) pulsemetric.CachedInspectPort {
	return p.port
}
func (p workflowMetricProvider) Open(context.Context, string, string, string) (pulsemetric.ReadSession, error) {
	return pulsemetric.ReadSession{ReadTarget: p.target, List: p.port, Inspect: p.port}, nil
}
func (workflowMetricProvider) CacheSetupError(_, _ string, err error) error { return err }
func (workflowMetricProvider) Now() time.Time                               { return time.Now() }

func delete(ctx context.Context, reader pulsemetric.DeleteReader, deleter pulsemetric.Deleter, input pulsemetric.DeleteInput) (pulsemetric.DeleteOutput, error) {
	if err := pulsemetric.DeleteValidateInput(input); err != nil {
		return pulsemetric.DeleteOutput{}, err
	}
	return pulsemetric.Delete(ctx, reader, deleter, input)
}

func fork(ctx context.Context, reader pulsemetric.ForkReader, creator pulsemetric.ForkCreator, reconciler pulsemetric.ForkReconciler, input pulsemetric.ForkInput, preview bool) (pulsemetric.ForkOutput, error) {
	if err := pulsemetric.ForkValidateInput(&input); err != nil {
		return pulsemetric.ForkOutput{}, err
	}
	return pulsemetric.Fork(ctx, reader, creator, reconciler, input, preview)
}

func followers(ctx context.Context, reader pulsemetric.FollowersReader, input pulsemetric.FollowersInput) (pulsemetric.FollowersOutput, error) {
	if err := pulsemetric.FollowersValidateInput(input); err != nil {
		return pulsemetric.FollowersOutput{}, err
	}
	return pulsemetric.Followers(ctx, reader, input)
}

func follow(ctx context.Context, resolver pulsemetric.FollowResolver, creator pulsemetric.FollowCreator, input pulsemetric.FollowInput, preview bool) (pulsemetric.FollowOutput, error) {
	if err := pulsemetric.FollowValidateInput(input); err != nil {
		return pulsemetric.FollowOutput{}, err
	}
	return pulsemetric.Follow(ctx, resolver, creator, input, preview)
}

func unfollow(ctx context.Context, reader pulsemetric.UnfollowReader, deleter pulsemetric.UnfollowDeleter, input pulsemetric.UnfollowInput, preview bool) (pulsemetric.UnfollowOutput, error) {
	if err := pulsemetric.UnfollowValidateInput(input); err != nil {
		return pulsemetric.UnfollowOutput{}, err
	}
	return pulsemetric.Unfollow(ctx, reader, deleter, input, preview)
}
