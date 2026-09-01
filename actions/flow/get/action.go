package get

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned flow seam.
type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
}
type Action struct{ resolver Resolver }

func New(resolver Resolver) *Action { return &Action{resolver: resolver} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, errors.New("flow resolver is not configured")
	}
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return Output{}, errors.New("flow selection requires a LUID or exact name and project path")
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Flow: flow, RequestID: flow.RequestID, Help: []string{"tadx content flow pull --id " + flow.LUID}}, nil
}
