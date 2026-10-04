package project

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

// Target is the canonical environment and exact site selected for one command.
type Target struct {
	Environment string
	Site        string
}

// CachedListReader supplies a cache page and the source observed during the read.
type CachedListReader interface {
	ListReader
	Source() *readsource.Metadata
}

// CachedInspectResolver supplies a cached identity and its coverage metadata.
type CachedInspectResolver interface {
	InspectResolver
	Source() *readsource.Metadata
}

// CollectedList is one complete live inventory prepared by the shared collector.
type CollectedList struct {
	Reader    ListReader
	Source    *readsource.Metadata
	RequestID string
	Help      string
}

// InventoryPort supplies shared collection and cache publication mechanisms.
type InventoryPort interface {
	CollectProjects(context.Context, string, time.Time) (CollectedList, error)
	PublishProjectInspect(context.Context, InspectOutput, time.Time)
}

// LiveSession binds one authenticated target to direct project action ports.
type LiveSession struct {
	Target
	Ports
	Inventory InventoryPort
}

// Provider constructs cache and authenticated project ports when a command needs them.
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

// ListProjects selects a cached page, live page, or complete live inventory.
func (a *Service) ListProjects(ctx context.Context, input ListInput) (result ListOutput, resultErr error) {
	if a == nil || a.Provider == nil {
		return ListOutput{}, errs.New(errs.KindRuntime, "project list provider is not configured")
	}
	if input.Cursor != "" {
		target, err := a.Provider.CacheTarget(input.Environment)
		if err != nil {
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
	}
	selected, err := listParseInput(input)
	if err != nil {
		return ListOutput{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = a.Provider.ValidateComplete(input.All, result.Source)
		}
	}()
	if input.Cache || a.Provider.LegacyInventoryCursor(input.Cursor) {
		target, err := a.Provider.CacheTarget(input.Environment)
		if err != nil {
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		if err := selected.bindTarget(input); err != nil {
			return ListOutput{}, err
		}
		reader := a.Provider.CachedList(target)
		output, err := (&runner{Ports: Ports{ListReader: reader}}).listValidated(ctx, input, selected)
		if err == nil {
			output.Source = reader.Source()
		}
		return output, err
	}
	filter, err := a.Provider.ListFilter(input)
	if err != nil {
		return ListOutput{}, err
	}
	session, err := a.Provider.Open(ctx, input.Environment, input.Site, "project.list", false)
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if err := selected.bindTarget(input); err != nil {
		return ListOutput{}, err
	}
	if input.All && !ListMayBeOwnerLUID(input.OwnerName) {
		observedAt := a.Provider.Now().UTC()
		inventory, err := session.Inventory.CollectProjects(ctx, filter, observedAt)
		if err != nil {
			return ListOutput{}, err
		}
		output, err := (&runner{Ports: Ports{ListReader: inventory.Reader}}).listValidated(ctx, input, selected)
		if err != nil {
			return output, err
		}
		output.Source = inventory.Source
		output.RequestID = inventory.RequestID
		if inventory.Help != "" {
			output.Help = append(output.Help, inventory.Help)
		}
		return output, nil
	}
	output, err := (&runner{Ports: Ports{ListReader: session.ListReader}}).listValidated(ctx, input, selected)
	if err != nil {
		return output, err
	}
	output.Source = liveSource(a.Provider.Now)
	return output, nil
}

// InspectProject selects a cached or live identity and publishes successful live detail.
func (a *Service) InspectProject(ctx context.Context, input InspectInput) (InspectOutput, error) {
	if err := ValidateInspectInput(input); err != nil {
		return InspectOutput{}, err
	}
	if a == nil || a.Provider == nil {
		return InspectOutput{}, errs.New(errs.KindRuntime, "project inspect provider is not configured")
	}
	if input.Cache {
		target, err := a.Provider.CacheTarget(input.Environment)
		if err != nil {
			return InspectOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		resolver := a.Provider.CachedInspect(target)
		output, err := (&runner{Ports: Ports{InspectResolver: resolver}}).inspectValidated(ctx, input)
		if err == nil {
			output.Source = resolver.Source()
		}
		return output, err
	}
	session, err := a.Provider.Open(ctx, input.Environment, input.Site, "project.inspect", false)
	if err != nil {
		return InspectOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := (&runner{Ports: Ports{InspectResolver: session.InspectResolver}}).inspectValidated(ctx, input)
	if err != nil {
		return output, err
	}
	observedAt := a.Provider.Now().UTC()
	output.Source = liveSource(a.Provider.Now)
	session.Inventory.PublishProjectInspect(ctx, output, observedAt)
	return output, nil
}

// CreateProject validates input before opening a mutation-capable target.
func (a *Service) CreateProject(ctx context.Context, input CreateInput, preview bool) (CreateOutput, error) {
	if err := ValidateCreateInput(input); err != nil {
		return CreateOutput{}, err
	}
	if a == nil || a.Provider == nil {
		return CreateOutput{}, errs.New(errs.KindRuntime, "project create provider is not configured")
	}
	session, err := a.Provider.Open(ctx, input.Environment, input.Site, "project.create", true)
	if err != nil {
		return CreateOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	out, err := (&runner{Ports: Ports{CreateResolver: session.CreateResolver, Creator: session.Creator}}).createValidated(ctx, input, preview)
	if err == nil && out.Result != nil && out.Result.Project.Path == "" {
		out.Help = append(out.Help, MutationPathWarning)
	}
	return out, err
}

// UpdateProject validates input before opening a mutation-capable target.
func (a *Service) UpdateProject(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if err := ValidateUpdateInput(input); err != nil {
		return UpdateOutput{}, err
	}
	if a == nil || a.Provider == nil {
		return UpdateOutput{}, errs.New(errs.KindRuntime, "project update provider is not configured")
	}
	session, err := a.Provider.Open(ctx, input.Environment, input.Site, "project.update", true)
	if err != nil {
		return UpdateOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	out, err := (&runner{Ports: Ports{UpdateResolver: session.UpdateResolver, Updater: session.Updater}}).updateValidated(ctx, input, preview)
	if err == nil && out.Result != nil && out.Result.Project.Path == "" {
		out.Help = append(out.Help, MutationPathWarning)
	}
	return out, err
}

// DeleteProject validates the exact target before opening a mutation-capable session.
func (a *Service) DeleteProject(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if a == nil || a.Provider == nil {
		return DeleteOutput{}, errs.New(errs.KindRuntime, "project delete provider is not configured")
	}
	session, err := a.Provider.Open(ctx, input.Environment, input.Site, "project.delete", true)
	if err != nil {
		return DeleteOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	return (&runner{Ports: Ports{DeleteResolver: session.DeleteResolver, Deleter: session.Deleter}}).deleteValidated(ctx, input, preview)
}

// MoveProject validates selectors before opening a mutation-capable session.
func (a *Service) MoveProject(ctx context.Context, input MoveInput, preview bool) (MoveOutput, error) {
	if err := ValidateMoveInput(input); err != nil {
		return MoveOutput{}, err
	}
	if a == nil || a.Provider == nil {
		return MoveOutput{}, errs.New(errs.KindRuntime, "project move provider is not configured")
	}
	session, err := a.Provider.Open(ctx, input.Environment, input.Site, "project.move", true)
	if err != nil {
		return MoveOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	out, err := (&runner{Ports: Ports{MoveResolver: session.MoveResolver, Mover: session.Mover}}).moveValidated(ctx, input, preview)
	if err == nil && out.Result != nil && out.Result.Project.Path == "" {
		out.Help = append(out.Help, MutationPathWarning)
	}
	return out, err
}

const MutationPathWarning = "Project mutation succeeded, but its canonical hierarchy path could not be confirmed; inspect the project by LUID."

func liveSource(now func() time.Time) *readsource.Metadata {
	value := readsource.Live(now().UTC())
	return &value
}
