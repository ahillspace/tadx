package datasource

import (
	"context"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// ResolvePublicationDestination reads the saved exact identity or unique name/project pair.
func (a *Adapter) ResolvePublicationDestination(ctx context.Context, input value.PublicationDestination) (value.ResourceDestination, error) {
	fallback := value.ResourceDestination{ResourceID: input.ResourceID, Name: input.Name, ProjectID: input.ProjectID}
	if input.ResourceID != "" {
		item, err := a.ResolveDatasource(ctx, identity.Selector{LUID: identity.LUID(input.ResourceID)})
		if err != nil {
			return fallback, err
		}
		return value.ResourceDestination{ResourceID: item.LUID, Name: item.Name, ProjectID: item.ProjectLUID}, nil
	}
	if input.Name == "" || input.ProjectID == "" {
		return fallback, errors.New("receipt has no exact datasource name and project")
	}
	matches, err := a.FindDatasources(ctx, input.Name, input.ProjectID)
	if err != nil {
		return fallback, err
	}
	if len(matches) == 0 {
		return fallback, nil
	}
	if len(matches) != 1 || matches[0].Name != input.Name || matches[0].ProjectLUID != input.ProjectID {
		return fallback, fmt.Errorf("exact datasource destination is ambiguous or unavailable")
	}
	return value.ResourceDestination{ResourceID: matches[0].LUID, Name: matches[0].Name, ProjectID: matches[0].ProjectLUID}, nil
}
