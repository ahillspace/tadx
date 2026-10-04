package group

import (
	"context"
	"errors"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

// Target identifies the canonical environment and exact site selected for one command.
type Target struct{ Environment, Site string }

// CachedListReader provides a cached page and its observed source facts.
type CachedListReader interface {
	ListReader
	Source() *readsource.Metadata
}

// CachedInspectResolver provides cached detail and its observed source facts.
type CachedInspectResolver interface {
	Resolver
	Source() *readsource.Metadata
}

// LivePorts binds all group operations to one authenticated target.
type LivePorts interface {
	ListReader
	Resolver
	CreateFinder
	CreateWriter
	UpdateWriter
	MembershipWriter
	DeleteWriter
	MembershipResolver
	MembershipAddWriter
	MembershipRemoveWriter
}

// CollectedList is one complete live group inventory prepared by the shared collector.
type CollectedList struct {
	Reader    ListReader
	Source    *readsource.Metadata
	RequestID string
	Help      string
}

// InventoryPort provides shared inventory and cache-publication mechanisms.
type InventoryPort interface {
	CollectGroups(context.Context, string, time.Time) (CollectedList, error)
	PublishGroupInspect(context.Context, InspectOutput, bool, time.Time)
}

// LiveSession binds canonical target identity to resource-owned action ports.
type LiveSession struct {
	Target
	Ports     LivePorts
	Inventory InventoryPort
}

// Provider opens only the cache or live ports needed after local validation.
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

// Service owns group lifecycle and membership decisions.
type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }

func (s *Service) ListAdminGroups(ctx context.Context, input ListInput) (result ListOutput, resultErr error) {
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
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.group.list", false)
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if input.All {
		observedAt := s.provider.Now().UTC()
		inventory, err := session.Inventory.CollectGroups(ctx, filter, observedAt)
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

func (s *Service) InspectAdminGroup(ctx context.Context, input InspectInput) (InspectOutput, error) {
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
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.group.inspect", false)
	if err != nil {
		return InspectOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := Inspect(ctx, session.Ports, input)
	if err != nil {
		return output, groupActionError("admin.group.inspect", input.Environment, input.Site, err)
	}
	observedAt := s.provider.Now().UTC()
	value := readsource.Live(s.provider.Now().UTC())
	output.Source = &value
	session.Inventory.PublishGroupInspect(ctx, output, input.IncludeMembers, observedAt)
	return output, nil
}

func (s *Service) CreateAdminGroup(ctx context.Context, input CreateInput, preview bool) (CreateOutput, error) {
	if err := ValidateCreateInput(input); err != nil {
		return CreateOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.group.create", true)
	if err != nil {
		return CreateOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := Create(ctx, session.Ports, session.Ports, input, preview)
	return output, groupActionError("admin.group.create", input.Environment, input.Site, err)
}

func (s *Service) UpdateAdminGroup(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if err := ValidateUpdateInput(&input); err != nil {
		return UpdateOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.group.update", true)
	if err != nil {
		return UpdateOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := Update(ctx, session.Ports, session.Ports, session.Ports, input, preview)
	return output, groupActionError("admin.group.update", input.Environment, input.Site, err)
}

func (s *Service) DeleteAdminGroup(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.group.delete", true)
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := Delete(ctx, session.Ports, session.Ports, input, preview)
	return output, groupActionError("admin.group.delete", input.Environment, input.Site, err)
}

func (s *Service) AddAdminGroupMember(ctx context.Context, input MembershipInput, preview bool) (MembershipOutput, error) {
	if err := ValidateAddMemberInput(input); err != nil {
		return MembershipOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.group.member.add", true)
	if err != nil {
		return MembershipOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := AddMember(ctx, session.Ports, session.Ports, input, preview)
	return output, groupActionError("admin.group.member.add", input.Environment, input.Site, err)
}

func (s *Service) RemoveAdminGroupMember(ctx context.Context, input MembershipInput, preview bool) (MembershipOutput, error) {
	if err := ValidateRemoveMemberInput(input); err != nil {
		return MembershipOutput{}, err
	}
	session, err := s.provider.Open(ctx, input.Environment, input.Site, "admin.group.member.remove", true)
	if err != nil {
		return MembershipOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := RemoveMember(ctx, session.Ports, session.Ports, input, preview)
	return output, groupActionError("admin.group.member.remove", input.Environment, input.Site, err)
}

func groupActionError(operation, environment, site string, err error) error {
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
	return &errs.Error{ID: "group.list.inventory_refresh_failed", Kind: errs.KindOperation, Operation: "group.list", Environment: input.Environment, Site: input.Site, Summary: "Complete live inventory refresh failed.", Cause: err, Retryable: retryable, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(err)}
}
