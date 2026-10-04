package permission

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// Target identifies the canonical environment and exact site for one operation.
type Target struct{ Environment, Site string }

// LivePorts isolates exact principal selection and native permission records.
type LivePorts interface {
	InspectReader
	Reader
	CreateWriter
	DeleteWriter
	ResolvePrincipalUsername(context.Context, string) (string, error)
}

// LiveSession binds permission ports to one authenticated target.
type LiveSession struct {
	Target
	Ports LivePorts
}

// Provider opens a target-bound permission session only after local validation.
type Provider interface {
	Open(context.Context, string, string, string, bool) (LiveSession, error)
}

// Service owns permission inspection and mutation sequencing.
type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }

func (s *Service) InspectAdminPermission(ctx context.Context, in InspectInput) (InspectOutput, error) {
	if err := ValidateInspectInput(in); err != nil {
		return InspectOutput{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, in.Site, "admin.permission.inspect", false)
	if err != nil {
		return InspectOutput{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	if in.PrincipalUsername != "" {
		luid, err := session.Ports.ResolvePrincipalUsername(ctx, in.PrincipalUsername)
		if err != nil {
			return InspectOutput{}, permissionActionError("admin.permission.inspect", in.Environment, in.Site, err)
		}
		in.PrincipalLUID, in.PrincipalUsername = luid, ""
	}
	output, err := InspectPermissions(ctx, session.Ports, in)
	return output, permissionActionError("admin.permission.inspect", in.Environment, in.Site, err)
}

func (s *Service) CreateAdminPermission(ctx context.Context, in Input, preview bool) (Output, error) {
	if err := ValidateCreateInput(in); err != nil {
		return Output{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, in.Site, createOperation, true)
	if err != nil {
		return Output{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	if in.PrincipalUsername != "" {
		luid, err := session.Ports.ResolvePrincipalUsername(ctx, in.PrincipalUsername)
		if err != nil {
			return Output{}, permissionActionError(createOperation, in.Environment, in.Site, err)
		}
		in.PrincipalLUID, in.PrincipalUsername = luid, ""
	}
	output, err := Create(ctx, session.Ports, session.Ports, in, preview)
	return output, permissionMutationError(createOperation, in.Environment, in.Site, err)
}

func (s *Service) DeleteAdminPermission(ctx context.Context, in Input, preview bool) (Output, error) {
	if err := ValidateDeleteInput(in); err != nil {
		return Output{}, err
	}
	session, err := s.provider.Open(ctx, in.Environment, in.Site, deleteOperation, true)
	if err != nil {
		return Output{}, err
	}
	in.Environment, in.Site = session.Environment, session.Site
	if in.PrincipalUsername != "" {
		luid, err := session.Ports.ResolvePrincipalUsername(ctx, in.PrincipalUsername)
		if err != nil {
			return Output{}, permissionActionError(deleteOperation, in.Environment, in.Site, err)
		}
		in.PrincipalLUID, in.PrincipalUsername = luid, ""
	}
	output, err := Delete(ctx, session.Ports, session.Ports, in, preview)
	return output, permissionMutationError(deleteOperation, in.Environment, in.Site, err)
}

func permissionActionError(operation, environment, site string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*errs.Error](err); ok {
		return err
	}
	retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact administration target and upstream response, then retry.")
	return &errs.Error{ID: operation + ".failed", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: "Tableau administration operation failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(err)}
}

type principalResolution interface {
	error
	PrerequisiteKind() string
	PrerequisiteResource() string
	PrerequisiteSummary() string
}

func permissionMutationError(operation, environment, site string, err error) error {
	if err == nil {
		return nil
	}
	resolution, ok := errors.AsType[principalResolution](err)
	if !ok {
		return permissionActionError(operation, environment, site, err)
	}
	retryable, _ := errs.CompleteRetryAdvice(err, "Review the exact principal before retrying.")
	principalType, principalLUID := resolution.PrerequisiteKind(), resolution.PrerequisiteResource()
	hint := commandhint.Environment(environment, "admin", principalType, "inspect", "--id", principalLUID)
	return &errs.Error{ID: operation + ".principal.resolve", Kind: errs.KindOperation, Operation: operation, Resource: principalLUID, Environment: environment, Site: site, Summary: "Permission principal resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: "Review the requested principal type and identity. Run " + hint + ", then create a new preview before any write.", TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseVerification, Outcome: errs.OutcomeNotAttempted, Prerequisite: &errs.Prerequisite{Kind: principalType, Resource: principalLUID, Summary: resolution.PrerequisiteSummary()}}
}
