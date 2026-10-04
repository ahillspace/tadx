package datasource

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
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
type UpstreamObservation struct {
	Databases  []value.MetadataDatabase
	Tables     []value.MetadataTable
	Complete   bool
	ObservedAt string
}
type UpstreamReader interface {
	ReadDatasourceUpstream(context.Context, string) (UpstreamObservation, error)
}
type ReadInventory interface {
	CollectDatasources(context.Context, string, time.Time) (CollectedList, error)
	PublishDatasourceInspect(context.Context, InspectOutput, time.Time)
}
type ReadSession struct {
	ReadTarget
	Reader    ListReader
	Resolver  Resolver
	Upstream  UpstreamReader
	Inventory ReadInventory
}
type ReadProvider interface {
	CacheTarget(string) (ReadTarget, error)
	CachedList(ReadTarget) CachedListReader
	CachedInspect(ReadTarget) CachedInspectResolver
	LegacyInventoryCursor(string) bool
	ListFilter(ListInput) (string, error)
	OpenDatasourceRead(context.Context, string, string, string) (ReadSession, error)
	ValidateComplete(bool, *readsource.Metadata) error
	RefreshError(string, string, string, error) error
	Now() time.Time
}

func (s *Service) ListDatasources(ctx context.Context, input ListInput) (result ListOutput, resultErr error) {
	if s == nil || s.ports.Read == nil {
		return ListOutput{}, errs.New(errs.KindRuntime, "datasource read provider is not configured")
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
			resultErr = p.ValidateComplete(input.All, result.Source)
		}
	}()
	if input.Cache || p.LegacyInventoryCursor(input.Cursor) {
		target, err := p.CacheTarget(input.Environment)
		if err != nil {
			return ListOutput{}, err
		}
		input.Environment, input.Site = target.Environment, target.Site
		if input.Cursor == "" && !input.All {
			selection.Filter, err = listDatasourceFilterFingerprint(input)
			if err != nil {
				return ListOutput{}, err
			}
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
	session, err := p.OpenDatasourceRead(ctx, input.Environment, input.Site, "datasource.list")
	if err != nil {
		return ListOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	if input.Cursor == "" && !input.All {
		selection.Filter, err = listDatasourceFilterFingerprint(input)
		if err != nil {
			return ListOutput{}, err
		}
	}
	if input.All && input.ProjectLUID != "" {
		output, err := listValidated(ctx, session.Reader, input, selection)
		if err != nil {
			return output, err
		}
		value := readsource.Live(p.Now().UTC())
		output.Source = &value
		return output, nil
	}
	if input.All {
		observedAt := p.Now().UTC()
		collected, err := session.Inventory.CollectDatasources(ctx, filter, observedAt)
		if err != nil {
			return ListOutput{}, p.RefreshError("datasource.list", input.Environment, input.Site, err)
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

func (s *Service) InspectDatasource(ctx context.Context, input InspectInput) (InspectOutput, error) {
	var err error
	if input, err = inspectNormalizeInput(input); err != nil {
		return InspectOutput{}, err
	}
	if s == nil || s.ports.Read == nil {
		return InspectOutput{}, errs.New(errs.KindRuntime, "datasource read provider is not configured")
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
	session, err := p.OpenDatasourceRead(ctx, input.Environment, input.Site, "datasource.inspect")
	if err != nil {
		return InspectOutput{}, err
	}
	input.Environment, input.Site = session.Environment, session.Site
	output, err := inspectValidated(ctx, session.Resolver, input)
	if err != nil {
		return output, err
	}
	upstream, upstreamErr := session.Upstream.ReadDatasourceUpstream(ctx, output.Datasource.LUID)
	if upstreamErr != nil {
		output.Datasource.Upstream = &Upstream{Status: "unavailable", Help: commandhint.Environment(input.Environment, "catalog", "audit", "--type", "datasource", "--id", output.Datasource.LUID)}
	} else {
		output.Datasource.Upstream = &Upstream{Status: "observed", Databases: upstream.Databases, Tables: upstream.Tables, Complete: upstream.Complete, ObservedAt: upstream.ObservedAt}
	}
	observedAt := p.Now().UTC()
	value := readsource.Live(p.Now().UTC())
	output.Source = &value
	session.Inventory.PublishDatasourceInspect(ctx, output, observedAt)
	return output, nil
}
