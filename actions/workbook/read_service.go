package workbook

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
)

type ReadTarget struct{ Environment, Site string }

type CachedListReader interface {
	ListReader
	Source() *readsource.Metadata
}

type CachedInspectResolver interface {
	Resolver
	Source() *readsource.Metadata
}

type CollectedList struct {
	Reader          ListReader
	Source          *readsource.Metadata
	RequestID, Help string
}

type ReadInventory interface {
	CollectWorkbooks(context.Context, string, time.Time) (CollectedList, error)
	CollectProjectWorkbooks(context.Context, ListInput) (ListReader, error)
	PublishWorkbookInspect(context.Context, InspectOutput, time.Time)
}

type ReadSession struct {
	ReadTarget
	Reader    ListReader
	Resolver  Resolver
	Inventory ReadInventory
}

type ReadProvider interface {
	CacheTarget(string) (ReadTarget, error)
	CachedList(ReadTarget) CachedListReader
	CachedInspect(ReadTarget) CachedInspectResolver
	ListFilter(ListInput) (string, error)
	OpenWorkbookRead(context.Context, string, string, string) (ReadSession, error)
	Now() time.Time
}

func (s *Service) ListWorkbooks(ctx context.Context, input ListInput) (result ListOutput, resultErr error) {
	if s == nil || s.ports.Read == nil {
		return ListOutput{}, errs.New(errs.KindRuntime, "workbook read provider is not configured")
	}
	p := s.ports.Read
	if input.Cursor != "" {
		target, err := p.CacheTarget(input.Environment)
		if err != nil {
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
	}
	selection, err := listValidateInput(input)
	if err != nil {
		return ListOutput{}, err
	}
	defer func() {
		if resultErr == nil {
			resultErr = inventory.ValidateAll(input.All, result.Source)
		}
	}()
	if input.Cache || inventory.LegacyInventoryCursor(input.Cursor) {
		target, err := p.CacheTarget(input.Environment)
		if err != nil {
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		if input.Cursor == "" && !input.All {
			selection.Filter = listFilterFingerprint(input)
		}
		reader := p.CachedList(target)
		output, err := listValidated(ctx, reader, input, selection)
		if err == nil {
			output.Source = reader.Source()
		}
		return output, err
	}
	filter, err := p.ListFilter(input)
	if err != nil {
		return ListOutput{}, err
	}
	session, err := p.OpenWorkbookRead(ctx, input.Environment, input.Site, "workbook.list")
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if input.Cursor == "" && !input.All {
		selection.Filter = listFilterFingerprint(input)
	}
	if input.All && input.ProjectLUID != "" {
		reader, err := session.Inventory.CollectProjectWorkbooks(ctx, input)
		if err != nil {
			return ListOutput{}, inventory.RefreshError("workbook.list", input.Environment, input.Site, err)
		}
		output, err := listValidated(ctx, reader, input, selection)
		if err != nil {
			return output, err
		}
		value := readsource.Live(p.Now().UTC())
		output.Source = &value
		return output, nil
	}
	if input.All {
		observedAt := p.Now().UTC()
		collected, err := session.Inventory.CollectWorkbooks(ctx, filter, observedAt)
		if err != nil {
			return ListOutput{}, inventory.RefreshError("workbook.list", input.Environment, input.Site, err)
		}
		output, err := listValidated(ctx, collected.Reader, input, selection)
		if err != nil {
			return output, err
		}
		output.Source, output.RequestID = collected.Source, collected.RequestID
		if collected.Help != "" {
			output.Help = append(output.Help, collected.Help)
		}
		return output, nil
	}
	output, err := listValidated(ctx, session.Reader, input, selection)
	if err != nil {
		return output, err
	}
	value := readsource.Live(p.Now().UTC())
	output.Source = &value
	return output, nil
}

func (s *Service) InspectWorkbook(ctx context.Context, input InspectInput) (InspectOutput, error) {
	var err error
	if input, err = inspectNormalizeInput(input); err != nil {
		return InspectOutput{}, err
	}
	if s == nil || s.ports.Read == nil {
		return InspectOutput{}, errs.New(errs.KindRuntime, "workbook read provider is not configured")
	}
	p := s.ports.Read
	if input.Cache {
		target, err := p.CacheTarget(input.Environment)
		if err != nil {
			return InspectOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		resolver := p.CachedInspect(target)
		output, err := inspectValidated(ctx, resolver, input)
		if err == nil {
			output.Source = resolver.Source()
		}
		return output, err
	}
	session, err := p.OpenWorkbookRead(ctx, input.Environment, input.Site, "workbook.inspect")
	if err != nil {
		return InspectOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := inspectValidated(ctx, session.Resolver, input)
	if err != nil {
		return output, err
	}
	observedAt := p.Now().UTC()
	value := readsource.Live(p.Now().UTC())
	output.Source = &value
	session.Inventory.PublishWorkbookInspect(ctx, output, observedAt)
	return output, nil
}
