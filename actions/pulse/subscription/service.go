package subscription

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/readsource"
)

// Session contains the canonical target and authenticated user's read port.
type Session struct {
	Environment string
	Site        string
	UserLUID    string
	Reader      Reader
}

// Provider opens the command-scoped authenticated reader after local validation.
type Provider interface {
	Open(context.Context, string, string) (Session, error)
}

// Service owns subscription discovery and its target-bound observations.
type Service struct {
	provider Provider
	now      func() time.Time
}

func New(provider Provider, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{provider: provider, now: now}
}

func (s *Service) ListPulseSubscriptions(ctx context.Context, input Input) (Output, error) {
	limit, err := validatedLimit(input)
	if err != nil {
		return Output{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site)
	if err != nil {
		return Output{}, err
	}
	input.Environment, input.Site, input.UserLUID = session.Environment, session.Site, session.UserLUID
	output, err := listValidated(ctx, session.Reader, input, limit)
	output.Source = new(readsource.Live(s.now()))
	return output, err
}
