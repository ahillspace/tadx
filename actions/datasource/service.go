package datasource

import (
	"context"

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

type MutationProvider interface {
	OpenDatasourceMutation(context.Context, string, string, string) (MutationSession, error)
}

type Ports struct {
	Mutation MutationProvider
	Read     ReadProvider
	Schema   SchemaProvider
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

func (s *Service) MoveDatasource(ctx context.Context, input MoveInput, preview bool) (MoveOutput, error) {
	if err := ValidateMoveInput(input); err != nil {
		return MoveOutput{}, err
	}
	if s == nil || s.ports.Mutation == nil {
		return MoveOutput{}, errs.New(errs.KindRuntime, "datasource mutation provider is not configured")
	}
	session, err := s.ports.Mutation.OpenDatasourceMutation(ctx, input.Environment, input.Site, "datasource.move")
	if err != nil {
		return MoveOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	return moveValidated(ctx, session.MoveResolver, session.Mover, input, preview)
}

func (s *Service) UpdateDatasource(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if err := ValidateUpdateInput(input); err != nil {
		return UpdateOutput{}, err
	}
	if s == nil || s.ports.Mutation == nil {
		return UpdateOutput{}, errs.New(errs.KindRuntime, "datasource mutation provider is not configured")
	}
	session, err := s.ports.Mutation.OpenDatasourceMutation(ctx, input.Environment, input.Site, "datasource.update")
	if err != nil {
		return UpdateOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	return updateValidated(ctx, session.UpdateResolver, session.Updater, input, preview)
}

func (s *Service) DeleteDatasource(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if s == nil || s.ports.Mutation == nil {
		return DeleteOutput{}, errs.New(errs.KindRuntime, "datasource mutation provider is not configured")
	}
	session, err := s.ports.Mutation.OpenDatasourceMutation(ctx, input.Environment, input.Site, "datasource.delete")
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	return deleteValidated(ctx, session.DeleteResolver, session.Deleter, input, preview)
}
