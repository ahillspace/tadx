package job

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/value"
)

type DestinationReader interface {
	ResolvePublicationDestination(context.Context, value.PublicationDestination) (value.ResourceDestination, error)
}

// Destinations binds resource-owned exact readback without submitting writes.
type Destinations struct{ Workbook, Datasource DestinationReader }

func (d Destinations) Resolve(ctx context.Context, input value.PublicationDestination) (value.ResourceDestination, error) {
	switch input.Operation {
	case "workbook.publish":
		return d.Workbook.ResolvePublicationDestination(ctx, input)
	case "datasource.publish":
		return d.Datasource.ResolvePublicationDestination(ctx, input)
	default:
		result := value.ResourceDestination{ResourceID: input.ResourceID, Name: input.Name, ProjectID: input.ProjectID}
		if input.ResourceID != "" {
			return result, nil
		}
		return result, fmt.Errorf("destination resolution is unsupported for operation %q", input.Operation)
	}
}
