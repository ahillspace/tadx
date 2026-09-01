package get

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned exact project seam.
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

// Action gets one exact project.
type Action struct{ resolver Resolver }

// New creates a project get action.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative project.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, errors.New("project resolver is not configured")
	}
	if input.Selector.LUID == "" && input.Selector.ProjectPath == "" {
		return Output{}, errors.New("project LUID or exact project path is required")
	}
	project, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Project: project, RequestID: project.RequestID, Help: []string{"tadx content project list"}}, nil
}
