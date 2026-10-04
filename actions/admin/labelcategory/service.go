package labelcategory

import "context"

// Ports binds the native definition contract for one selected target.
type Ports interface {
	ListReader
	InspectReader
	CreateReader
	CreateWriter
	UpdateReader
	UpdateWriter
	DeleteReader
	DeleteWriter
}

type Target struct{ Environment, Site string }
type LiveSession struct {
	Target
	Ports Ports
}

// Provider opens label-definition ports only after local input validation.
type Provider interface {
	Open(context.Context, string, string, bool) (LiveSession, error)
}

// Service owns the named shared category operations.
type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }

func (s *Service) ListLabelCategory(ctx context.Context, in ListInput) (ListOutput, error) {
	if err := ValidateListInput(in); err != nil {
		return ListOutput{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, "admin.label.category.list", false)
	if err != nil {
		return ListOutput{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	return List(ctx, session.Ports, in)
}

func (s *Service) InspectLabelCategory(ctx context.Context, in InspectInput) (InspectOutput, error) {
	if err := ValidateInspectInput(in); err != nil {
		return InspectOutput{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, "admin.label.category.inspect", false)
	if err != nil {
		return InspectOutput{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	return Inspect(ctx, session.Ports, in)
}

func (s *Service) CreateLabelCategory(ctx context.Context, in CreateInput, preview bool) (WriteOutput, error) {
	if err := ValidateCreateInput(in); err != nil {
		return WriteOutput{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, "admin.label.category.create", true)
	if err != nil {
		return WriteOutput{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	return Create(ctx, session.Ports, session.Ports, in, preview)
}

func (s *Service) UpdateLabelCategory(ctx context.Context, in UpdateInput, preview bool) (WriteOutput, error) {
	if err := ValidateUpdateInput(in); err != nil {
		return WriteOutput{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, "admin.label.category.update", true)
	if err != nil {
		return WriteOutput{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	return Update(ctx, session.Ports, session.Ports, in, preview)
}

func (s *Service) DeleteLabelCategory(ctx context.Context, in DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(in); err != nil {
		return DeleteOutput{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, "admin.label.category.delete", true)
	if err != nil {
		return DeleteOutput{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	return Delete(ctx, session.Ports, session.Ports, in, preview)
}
