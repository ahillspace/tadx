package contentlabel

import "context"

// Ports contains the native label contracts for one selected target.
type Ports interface {
	ListReader
	InspectReader
	UpdateReader
	UpdateWriter
	DeleteReader
	DeleteWriter
}

type Session struct {
	Environment string
	Site        string
	Ports       Ports
}

// Provider opens target-bound ports after local validation and prerequisite checks.
type Provider interface {
	Open(context.Context, string, string, bool) (Session, error)
}

// Service owns attachment operations, distinct from shared label definitions.
type Service struct {
	provider              Provider
	checkLabelValueAccess func() error
}

func New(provider Provider, checkLabelValueAccess func() error) *Service {
	return &Service{provider: provider, checkLabelValueAccess: checkLabelValueAccess}
}

func (s *Service) ListLabels(ctx context.Context, input ListInput) (ListOutput, error) {
	if err := ValidateListInput(input); err != nil {
		return ListOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, "content.label.list", false)
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return listLabels(ctx, session.Ports, input)
}

func (s *Service) InspectLabel(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if err := ValidateInspectInput(input); err != nil {
		return InspectOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, "content.label.inspect", false)
	if err != nil {
		return InspectOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return inspectLabel(ctx, session.Ports, input)
}

func (s *Service) UpdateLabel(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if err := ValidateUpdateInput(input); err != nil {
		return UpdateOutput{}, err
	}
	if err := s.checkLabelValueAccess(); err != nil {
		return UpdateOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, "content.label.update", true)
	if err != nil {
		return UpdateOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return updateLabel(ctx, session.Ports, session.Ports, input, preview)
}

func (s *Service) DeleteLabel(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if err := s.checkLabelValueAccess(); err != nil {
		return DeleteOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, "content.label.delete", true)
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	return deleteLabel(ctx, session.Ports, session.Ports, input, preview)
}
