package datasource

import (
	"context"
	"errors"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/value"
)

// FreshPublicationDestination binds a new target read for accepted completion.
type FreshPublicationDestination struct {
	Name, ProjectID string
	Open            func(context.Context) (*Adapter, error)
	Progress        func(context.Context)
}

func (p FreshPublicationDestination) Confirm(ctx context.Context, resourceID string) (string, string, string, error) {
	if p.Progress != nil {
		p.Progress(ctx)
	}
	adapter, err := p.Open(ctx)
	if err != nil {
		return resourceID, "", "", err
	}
	item, err := adapter.ConfirmPublicationDestination(ctx, resourceID, p.Name, p.ProjectID)
	return item.ResourceID, item.Name, item.ProjectID, err
}

// PublicationLifecycle converts a shared receipt into datasource completion facts.
type PublicationLifecycle struct {
	*jobmonitor.Publication
	Readback FreshPublicationDestination
}

func (p PublicationLifecycle) Wait(ctx context.Context, jobID string) (datasourceops.PublishObservation, error) {
	accepted := p.Base
	accepted.Observation.ID = jobID
	r, err := p.Publication.Wait(ctx, accepted)
	return datasourceops.PublishObservation{Status: r.Observation.Status, RequestID: r.Observation.RequestID, ResourceID: r.Observation.ResourceID}, err
}

func (p PublicationLifecycle) Destination(ctx context.Context, resourceID string) (string, string, string, error) {
	return p.Readback.Confirm(ctx, resourceID)
}

// ConfirmPublicationDestination verifies the accepted exact datasource identity.
func (a *Adapter) ConfirmPublicationDestination(ctx context.Context, resourceID, name, projectID string) (value.ResourceDestination, error) {
	item, err := a.ResolveDatasource(ctx, identity.Selector{LUID: identity.LUID(resourceID)})
	resolved := value.ResourceDestination{ResourceID: item.LUID, Name: item.Name, ProjectID: item.ProjectLUID}
	if resolved.ResourceID == "" {
		resolved.ResourceID = resourceID
	}
	if err == nil && (resolved.ResourceID != resourceID || resolved.Name != name || resolved.ProjectID != projectID) {
		err = errors.New("published destination does not match the accepted target")
	}
	return resolved, err
}
