package metric

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
)

type MutationSession struct {
	Environment, Site, SiteLUID string
	Fork                        interface {
		ForkReader
		ForkCreator
		ForkReconciler
	}
	Delete interface {
		DeleteReader
		Deleter
	}
}

type MutationProvider interface {
	OpenMetricMutation(context.Context, string, string, string) (MutationSession, error)
}

func (s *Service) ForkPulseMetric(ctx context.Context, input ForkInput, preview bool) (ForkOutput, error) {
	if err := forkValidateInput(&input); err != nil {
		return ForkOutput{}, err
	}
	if s == nil || s.ports.Mutation == nil {
		return ForkOutput{}, &errs.Error{ID: "pulse.metric.fork.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.metric.fork", Summary: "Pulse metric fork is not configured.", Retryable: errs.Bool(false)}
	}
	session, err := s.ports.Mutation.OpenMetricMutation(ctx, input.Environment, input.Site, "pulse.metric.fork")
	if err != nil {
		return ForkOutput{}, err
	}
	input.Environment, input.Site, input.SiteLUID = session.Environment, session.Site, session.SiteLUID
	return runFork(ctx, session.Fork, session.Fork, session.Fork, input, preview)
}

func (s *Service) DeletePulseMetric(ctx context.Context, input DeleteInput) (DeleteOutput, error) {
	if err := deleteValidateInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if s == nil || s.ports.Mutation == nil {
		return DeleteOutput{}, &errs.Error{ID: "pulse.metric.delete.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.metric.delete", Summary: "Pulse metric delete is not configured.", Retryable: errs.Bool(false)}
	}
	session, err := s.ports.Mutation.OpenMetricMutation(ctx, input.Environment, input.Site, "pulse.metric.delete")
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return runDelete(ctx, session.Delete, session.Delete, input)
}
