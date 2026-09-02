package project

import (
	"context"
	"errors"
	"strings"

	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

// MutationClient is the narrow released project mutation seam.
type MutationClient interface {
	Create(context.Context, tableauproject.CreateRequest) (tableauproject.MutationResult, error)
	Update(context.Context, tableauproject.UpdateRequest) (tableauproject.MutationResult, error)
}

// MutationAdapter validates exact project mutation inputs before delegation.
type MutationAdapter struct{ client MutationClient }

// NewMutationAdapter creates a project mutation adapter.
func NewMutationAdapter(client MutationClient) *MutationAdapter {
	return &MutationAdapter{client: client}
}

// CreateProject creates one project under an optional exact parent.
func (a *MutationAdapter) CreateProject(ctx context.Context, input tableauproject.CreateRequest) (tableauproject.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.Name) == "" {
		return tableauproject.MutationResult{}, errors.New("project create requires a configured client and name")
	}
	return a.client.Create(ctx, input)
}

// UpdateProject updates explicit bounded metadata on one exact project.
func (a *MutationAdapter) UpdateProject(ctx context.Context, input tableauproject.UpdateRequest) (tableauproject.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.LUID) == "" {
		return tableauproject.MutationResult{}, errors.New("project update requires a configured client and exact project LUID")
	}
	if input.Name == nil && input.Description == nil && input.ContentPermissions == nil {
		return tableauproject.MutationResult{}, errors.New("project update requires at least one explicit metadata field")
	}
	return a.client.Update(ctx, input)
}
