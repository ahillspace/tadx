package flow

import (
	"context"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	"github.com/ahillspace/tadx/internal/value"
)

// PublishPorts owns target-bound flow reads and native request translation.
type PublishPorts struct {
	Adapter  *Adapter
	Projects interface {
		ResolveProjectIdentity(context.Context, identity.Selector) (value.ProjectIdentity, error)
		BeginProjectResolution(context.Context) context.Context
	}
	Changes  *MutationAdapter
	Begin    func(context.Context, flow.PublishRequest) error
	Progress func(context.Context, string)
}

func (p PublishPorts) BeginProjectResolution(ctx context.Context) context.Context {
	return p.Projects.BeginProjectResolution(ctx)
}

func (p PublishPorts) ResolveProject(ctx context.Context, selector identity.Selector) (flow.Project, error) {
	return p.Projects.ResolveProjectIdentity(ctx, selector)
}

func (p PublishPorts) FindFlows(ctx context.Context, name, projectLUID string) ([]flow.Record, error) {
	return p.Adapter.FindFlows(ctx, name, projectLUID)
}

func (p PublishPorts) Prepare(ctx context.Context, input flow.PublishRequest) (flow.PreparedPublish, error) {
	if p.Begin != nil {
		if err := p.Begin(ctx, input); err != nil {
			return nil, err
		}
	}
	prepared, err := p.Changes.PrepareFlow(ctx, tableauflow.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Overwrite: input.Overwrite})
	if err != nil {
		return nil, err
	}
	return preparedFlowPublish{prepared: prepared, progress: p.Progress}, nil
}

type preparedFlowPublish struct {
	prepared tableauflow.PreparedPublish
	progress func(context.Context, string)
}

func (p preparedFlowPublish) Commit(ctx context.Context) (flow.PublishResult, error) {
	if p.progress != nil {
		p.progress(ctx, "Uploading and submitting flow")
	}
	result, err := p.prepared.Commit(ctx)
	return flow.PublishResult{Status: result.Status, FlowLUID: result.FlowLUID, FlowName: result.FlowName, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}
