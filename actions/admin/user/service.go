package user

import (
	"context"
	"errors"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

// Target identifies the canonical environment and exact site for one command.
type Target struct{ Environment, Site string }

type CachedListReader interface {
	ListReader
	Source() *readsource.Metadata
}

type CachedInspectResolver interface {
	Resolver
	Source() *readsource.Metadata
}

// LivePorts supplies target-bound user action contracts without app record copies.
type LivePorts interface {
	ListReader
	Resolver
	CreateFinder
	CreateWriter
	UpdateWriter
	DeleteWriter
	ResolveUsername(context.Context, string) (string, error)
}

type CollectedList struct {
	Reader    ListReader
	Source    *readsource.Metadata
	RequestID string
	Help      string
}

type InventoryPort interface {
	CollectUsers(context.Context, string, time.Time) (CollectedList, error)
	PublishUserInspect(context.Context, InspectOutput, time.Time)
}

type LiveSession struct {
	Target
	Ports     LivePorts
	Inventory InventoryPort
}

// Provider opens cache or authenticated ports only after local validation.
type Provider interface {
	CacheTarget(string) (Target, error)
	CachedList(Target) CachedListReader
	CachedInspect(Target) CachedInspectResolver
	LegacyInventoryCursor(string) bool
	ListFilter(ListInput) (string, error)
	Open(context.Context, string, string, string, bool) (LiveSession, error)
	ValidateComplete(bool, *readsource.Metadata) error
	Now() time.Time
}

// Service owns user list, detail, and mutation decisions.
type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }

func (s *Service) ListAdminUsers(ctx context.Context, input ListInput) (result ListOutput, resultErr error) {
	if err := ValidateListInput(&input); err != nil {
		return ListOutput{}, err
	}
	if input.Cursor != "" {
		target, err := s.provider.CacheTarget(input.Environment)
		if err != nil {
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		if err := ValidateListContinuation(&input); err != nil {
			return ListOutput{}, err
		}
	}
	defer func() {
		if resultErr == nil {
			resultErr = s.provider.ValidateComplete(input.All, result.Source)
		}
	}()
	if input.Cache || s.provider.LegacyInventoryCursor(input.Cursor) {
		target, err := s.provider.CacheTarget(input.Environment)
		if err != nil {
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		reader := s.provider.CachedList(target)
		output, err := List(ctx, reader, input)
		if err == nil {
			output.Source = reader.Source()
		}
		return output, err
	}
	filter, err := s.provider.ListFilter(input)
	if err != nil {
		return ListOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.user.list", false)
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if input.All {
		observedAt := s.provider.Now().UTC()
		inventory, err := session.Inventory.CollectUsers(ctx, filter, observedAt)
		if err != nil {
			return ListOutput{}, inventoryRefreshError(input, err)
		}
		output, err := List(ctx, inventory.Reader, input)
		if err != nil {
			return output, err
		}
		output.Source, output.RequestID = inventory.Source, inventory.RequestID
		if inventory.Help != "" {
			output.Help = append(output.Help, inventory.Help)
		}
		return output, nil
	}
	output, err := List(ctx, session.Ports, input)
	if err != nil {
		return output, err
	}
	value := readsource.Live(s.provider.Now().UTC())
	output.Source = &value
	return output, nil
}

func (s *Service) InspectAdminUser(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if err := ValidateInspectInput(input); err != nil {
		return InspectOutput{}, err
	}
	if input.Cache {
		target, err := s.provider.CacheTarget(input.Environment)
		if err != nil {
			return InspectOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		resolver := s.provider.CachedInspect(target)
		output, err := Inspect(ctx, resolver, input)
		if err == nil {
			output.Source = resolver.Source()
		}
		return output, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.user.inspect", false)
	if err != nil {
		return InspectOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := Inspect(ctx, session.Ports, input)
	if err != nil {
		return output, userActionError("admin.user.inspect", input.Environment, input.Site, err)
	}
	observedAt := s.provider.Now().UTC()
	value := readsource.Live(s.provider.Now().UTC())
	output.Source = &value
	session.Inventory.PublishUserInspect(ctx, output, observedAt)
	return output, nil
}

func (s *Service) CreateAdminUser(ctx context.Context, input CreateInput, preview bool) (CreateOutput, error) {
	if err := ValidateCreateInput(input); err != nil {
		return CreateOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.user.create", true)
	if err != nil {
		return CreateOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := Create(ctx, session.Ports, session.Ports, input, preview)
	return output, userActionError("admin.user.create", input.Environment, input.Site, err)
}

func (s *Service) UpdateAdminUser(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if err := ValidateUpdateInput(input); err != nil {
		return UpdateOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.user.update", true)
	if err != nil {
		return UpdateOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if input.Username != "" {
		luid, err := session.Ports.ResolveUsername(ctx, input.Username)
		if err != nil {
			return UpdateOutput{}, userActionError("admin.user.update", input.Environment, input.Site, err)
		}
		input.UserLUID, input.Username = luid, ""
	}
	output, err := Update(ctx, session.Ports, session.Ports, input, preview)
	return output, userActionError("admin.user.update", input.Environment, input.Site, err)
}

func (s *Service) DeleteAdminUser(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.user.delete", true)
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if input.Username != "" {
		luid, err := session.Ports.ResolveUsername(ctx, input.Username)
		if err != nil {
			return DeleteOutput{}, userActionError("admin.user.delete", input.Environment, input.Site, err)
		}
		input.UserLUID, input.Username = luid, ""
	}
	output, err := Delete(ctx, session.Ports, session.Ports, input, preview)
	return output, userActionError("admin.user.delete", input.Environment, input.Site, err)
}

func userActionError(operation, environment, site string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*errs.Error](err); ok {
		return err
	}
	retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact administration target and upstream response, then retry.")
	return &errs.Error{ID: operation + ".failed", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: "Tableau administration operation failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(err)}
}

func inventoryRefreshError(input ListInput, err error) error {
	retryable, advice := errs.CompleteRetryAdvice(err, "Retry the live list; the previous cache snapshot remains unchanged.")
	return &errs.Error{ID: "user.list.inventory_refresh_failed", Kind: errs.KindOperation, Operation: "user.list", Environment: input.Environment, Site: input.Site, Summary: "Complete live inventory refresh failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(err)}
}
