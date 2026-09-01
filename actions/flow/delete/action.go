package delete

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/identity"
)

type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
}
type Deleter interface {
	DeleteFlow(context.Context, string) (Result, error)
}
type Action struct {
	resolver Resolver
	deleter  Deleter
}

func New(resolver Resolver, deleter Deleter) *Action {
	return &Action{resolver: resolver, deleter: deleter}
}
func (a *Action) Execute(ctx context.Context, input Input, apply bool) (Output, error) {
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, errors.New("flow delete dependencies are not configured")
	}
	if input.Environment == "" || input.Site == "" {
		return Output{}, errors.New("flow delete requires an explicit resolved environment and site")
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		return Output{}, err
	}
	plan := Plan{Mode: "preview", Operation: "flow.delete", Environment: input.Environment, Site: input.Site, Target: flow}
	output := Output{Plan: plan, Help: []string{"Add --apply to delete this exact flow."}}
	if !apply {
		return output, nil
	}
	current, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		return Output{}, err
	}
	if current != flow {
		return Output{}, errors.New("flow delete target changed after preview")
	}
	result, err := a.deleter.DeleteFlow(ctx, flow.LUID)
	if err != nil {
		return Output{}, err
	}
	output.Applied = true
	output.Result = &result
	output.Help = []string{"tadx content flow list"}
	return output, nil
}
