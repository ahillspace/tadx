package metric_test

import (
	"context"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
)

// These helpers follow composition's input-validation boundary before invoking operations.
func list(ctx context.Context, reader pulsemetric.ListReader, input pulsemetric.ListInput) (pulsemetric.ListOutput, error) {
	if err := pulsemetric.ListValidateInput(&input); err != nil {
		return pulsemetric.ListOutput{}, err
	}
	if err := pulsemetric.ListValidateContinuation(input); err != nil {
		return pulsemetric.ListOutput{}, err
	}
	return pulsemetric.List(ctx, reader, input)
}

func inspect(ctx context.Context, reader pulsemetric.InspectReader, input pulsemetric.InspectInput) (pulsemetric.InspectOutput, error) {
	if err := pulsemetric.InspectValidateInput(input); err != nil {
		return pulsemetric.InspectOutput{}, err
	}
	return pulsemetric.Inspect(ctx, reader, input)
}

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
