package move

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/identity"
)

type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
	ResolveProject(context.Context, identity.Selector) (Project, error)
}
type Mover interface {
	MoveFlow(context.Context, string, string) (Result, error)
}
type Action struct {
	resolver Resolver
	mover    Mover
}

func New(resolver Resolver, mover Mover) *Action { return &Action{resolver: resolver, mover: mover} }
func (a *Action) Execute(ctx context.Context, input Input, apply bool) (Output, error) {
	if a == nil || a.resolver == nil || a.mover == nil {
		return Output{}, errors.New("flow move dependencies are not configured")
	}
	if input.Environment == "" || input.Site == "" {
		return Output{}, errors.New("flow move requires an explicit resolved environment and site")
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.FlowSelector)
	if err != nil {
		return Output{}, err
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		return Output{}, err
	}
	plan := Plan{Mode: "preview", Operation: "flow.move", Environment: input.Environment, Site: input.Site, Source: flow, Destination: project, NoOp: flow.ProjectLUID == project.LUID}
	output := Output{Plan: plan, Help: []string{"Add --apply to move this exact flow."}}
	if !apply {
		return output, nil
	}
	current, err := a.resolver.ResolveFlow(ctx, input.FlowSelector)
	if err != nil {
		return Output{}, err
	}
	destination, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		return Output{}, err
	}
	if current != flow || destination != project {
		return Output{}, errors.New("flow move source or destination changed after preview")
	}
	if plan.NoOp {
		output.Applied = true
		output.Result = &Result{Status: "unchanged", FlowLUID: flow.LUID, ProjectLUID: project.LUID}
		return output, nil
	}
	result, err := a.mover.MoveFlow(ctx, flow.LUID, project.LUID)
	if err != nil {
		return Output{}, err
	}
	output.Applied = true
	output.Result = &result
	output.Help = []string{"tadx content flow get --id " + flow.LUID}
	return output, nil
}
