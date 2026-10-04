package workbook

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type MutationSession struct {
	Environment    string
	Site           string
	MoveResolver   MoveResolver
	Mover          Mover
	UpdateResolver UpdateResolver
	Updater        Updater
	DeleteResolver Resolver
	Deleter        Deleter
}

// MutationProvider opens target-bound ports after caller input validation.
type MutationProvider interface {
	OpenWorkbookMutation(context.Context, string, string, string) (MutationSession, error)
}

// Service owns named workbook mutation operations.
type Service struct{ provider MutationProvider }

func New(provider MutationProvider) *Service { return &Service{provider: provider} }

func (s *Service) MoveWorkbook(ctx context.Context, input MoveInput, preview bool) (MoveOutput, error) {
	if err := ValidateMoveInput(input); err != nil {
		return MoveOutput{}, err
	}
	if s == nil || s.provider == nil {
		return MoveOutput{}, errs.New(errs.KindRuntime, "workbook mutation provider is not configured")
	}
	session, err := s.provider.OpenWorkbookMutation(ctx, input.Environment, input.Site, "workbook.move")
	if err != nil {
		return MoveOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	return moveValidated(ctx, session.MoveResolver, session.Mover, input, preview)
}

func (s *Service) UpdateWorkbook(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if err := ValidateUpdateInput(input); err != nil {
		return UpdateOutput{}, err
	}
	if s == nil || s.provider == nil {
		return UpdateOutput{}, errs.New(errs.KindRuntime, "workbook mutation provider is not configured")
	}
	session, err := s.provider.OpenWorkbookMutation(ctx, input.Environment, input.Site, "workbook.update")
	if err != nil {
		return UpdateOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	return updateValidated(ctx, session.UpdateResolver, session.Updater, input, preview)
}

func (s *Service) DeleteWorkbook(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	input, err := deleteNormalizeInput(input)
	if err != nil {
		return DeleteOutput{}, err
	}
	if strings.TrimSpace(input.Environment) == "" {
		return DeleteOutput{}, deleteUsage("environment", "workbook delete requires an explicit resolved environment and site")
	}
	if s == nil || s.provider == nil {
		return DeleteOutput{}, errs.New(errs.KindRuntime, "workbook mutation provider is not configured")
	}
	session, err := s.provider.OpenWorkbookMutation(ctx, input.Environment, input.Site, "workbook.delete")
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	return deleteValidated(ctx, session.DeleteResolver, session.Deleter, input, preview)
}
