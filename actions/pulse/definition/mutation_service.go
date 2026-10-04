package definition

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
)

type MutationSession struct {
	Environment, Site string
	Fields            CreateFieldValidator
	Definitions       interface {
		CreateCollisionFinder
		CreateCreator
		DeleteReader
		Deleter
	}
}

type MutationProvider interface {
	OpenDefinitionMutation(context.Context, string, string, string) (MutationSession, error)
}

func (s *Service) CreatePulseDefinition(ctx context.Context, input CreateInput, preview bool) (CreateOutput, error) {
	if err := createValidateInput(&input); err != nil {
		return CreateOutput{}, err
	}
	if s == nil || s.ports.Mutation == nil {
		return CreateOutput{}, &errs.Error{ID: "pulse.definition.create.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.definition.create", Summary: "Pulse definition create is not configured.", Retryable: errs.Bool(false)}
	}
	session, err := s.ports.Mutation.OpenDefinitionMutation(ctx, input.Environment, input.Site, "pulse.definition.create")
	if err != nil {
		return CreateOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return runCreate(ctx, session.Fields, session.Definitions, session.Definitions, input, preview)
}

func (s *Service) DeletePulseDefinition(ctx context.Context, input DeleteInput) (DeleteOutput, error) {
	if err := deleteValidateInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if s == nil || s.ports.Mutation == nil {
		return DeleteOutput{}, &errs.Error{ID: "pulse.definition.delete.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.definition.delete", Summary: "Pulse definition delete is not configured.", Retryable: errs.Bool(false)}
	}
	session, err := s.ports.Mutation.OpenDefinitionMutation(ctx, input.Environment, input.Site, "pulse.definition.delete")
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return runDelete(ctx, session.Definitions, session.Definitions, input)
}
