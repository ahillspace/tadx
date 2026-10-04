package flow

import (
	"context"
	"errors"
	"strings"

	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

// MutationClient is the temporary publication Prepare seam.
// The publication and recovery slice removes it after receipt fixtures cover that boundary.
type MutationClient interface {
	Prepare(context.Context, tableauflow.PublishRequest) (tableauflow.PreparedPublish, error)
}

// MutationAdapter validates publication preparation before delegation.
type MutationAdapter struct{ client MutationClient }

// NewMutationAdapter creates a flow mutation adapter.
func NewMutationAdapter(client MutationClient) *MutationAdapter {
	return &MutationAdapter{client: client}
}

// PrepareFlow prepares native bytes without performing the final publish.
func (a *MutationAdapter) PrepareFlow(ctx context.Context, input tableauflow.PublishRequest) (tableauflow.PreparedPublish, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.ProjectLUID) == "" {
		return nil, errors.New("flow publish requires a configured client, name, and project LUID")
	}
	return a.client.Prepare(ctx, input)
}
