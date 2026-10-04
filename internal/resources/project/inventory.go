package project

import (
	"context"
	"time"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

// InventoryPorts binds one project target to collection and detail publication.
type InventoryPorts struct {
	Executor          tableaucache.Executor
	Store             func(string) *cache.Store
	Environment, Site string
	MaxConcurrency    int
	Now               func() time.Time
}

func (p InventoryPorts) CollectProjects(ctx context.Context, filter string, observedAt time.Time) (projectops.CollectedList, error) {
	collected, err := inventory.Collect(ctx, p.Executor, p.Store(p.Environment), tableaucache.ScopeProjects, p.Environment, p.Site, observedAt, inventory.Options{MaxConcurrency: p.MaxConcurrency, Filter: filter})
	if err != nil {
		return projectops.CollectedList{}, inventory.RefreshError("project.list", p.Environment, p.Site, err)
	}
	requestID := collected.FinalRequestID()
	reader := InventoryListPort{Entries: collected.Entries, RequestID: requestID, AllowContinuation: true}
	source, help := collected.SourceAndHelp(observedAt, p.Now)
	return projectops.CollectedList{Reader: reader, RequestID: requestID, Source: source, Help: help}, nil
}

func (p InventoryPorts) PublishProjectInspect(_ context.Context, output projectops.InspectOutput, observedAt time.Time) {
	entry, err := InspectEntry(output, observedAt)
	if err == nil {
		inventory.PublishDetail(p.Store(output.Environment), entry)
	}
}
