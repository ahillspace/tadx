package flow

import (
	"context"
	"errors"
	"strings"

	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

// MutationClient is the narrow docs-only flow mutation seam.
type MutationClient interface {
	Prepare(context.Context, tableauflow.PublishRequest) (tableauflow.PreparedPublish, error)
	Move(context.Context, string, string) (tableauflow.MutationResult, error)
	Delete(context.Context, string) (tableauflow.MutationResult, error)
}

// MutationAdapter validates exact mutation identities before delegation.
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

// MoveFlow moves one exact flow to one exact project on the current site.
func (a *MutationAdapter) MoveFlow(ctx context.Context, flowLUID, projectLUID string) (tableauflow.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(flowLUID) == "" || strings.TrimSpace(projectLUID) == "" {
		return tableauflow.MutationResult{}, errors.New("flow move requires configured client and exact flow and project LUIDs")
	}
	return a.client.Move(ctx, flowLUID, projectLUID)
}

// DeleteFlow deletes one exact flow.
func (a *MutationAdapter) DeleteFlow(ctx context.Context, flowLUID string) (tableauflow.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(flowLUID) == "" {
		return tableauflow.MutationResult{}, errors.New("flow delete requires a configured client and exact flow LUID")
	}
	return a.client.Delete(ctx, flowLUID)
}
