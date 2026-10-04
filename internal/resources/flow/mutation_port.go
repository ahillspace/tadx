package flow

import (
	"context"
	"errors"
	"strings"

	flowops "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	"github.com/ahillspace/tadx/internal/value"
)

type FlowChanges interface {
	Update(context.Context, tableauflow.UpdateRequest) (tableauflow.MutationResult, error)
	Move(context.Context, string, string) (tableauflow.MutationResult, error)
	Delete(context.Context, string) (tableauflow.MutationResult, error)
}

type MutationProjectResolver interface {
	ResolveProjectIdentity(context.Context, identity.Selector) (value.ProjectIdentity, error)
	BeginProjectResolution(context.Context) context.Context
}

// MutationPort binds authoritative flow/project resolution to exact native writes.
type MutationPort struct {
	*Adapter
	projects MutationProjectResolver
	changes  FlowChanges
}

func NewMutationPort(reader *Adapter, projects MutationProjectResolver, changes FlowChanges) *MutationPort {
	return &MutationPort{Adapter: reader, projects: projects, changes: changes}
}

func (p *MutationPort) ResolveProject(ctx context.Context, selector identity.Selector) (flowops.Project, error) {
	if p == nil || p.projects == nil {
		return flowops.Project{}, errors.New("flow project resolver is not configured")
	}
	return p.projects.ResolveProjectIdentity(ctx, selector)
}

func (p *MutationPort) BeginProjectResolution(ctx context.Context) context.Context {
	return p.projects.BeginProjectResolution(ctx)
}

func (p *MutationPort) MoveFlow(ctx context.Context, flowLUID, projectLUID string) (flowops.MoveResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(flowLUID) == "" || strings.TrimSpace(projectLUID) == "" {
		return flowops.MoveResult{}, errors.New("flow move requires configured client and exact flow and project LUIDs")
	}
	result, err := p.changes.Move(ctx, flowLUID, projectLUID)
	return flowops.MoveResult{Status: result.Status, FlowLUID: result.FlowLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

func (p *MutationPort) UpdateFlow(ctx context.Context, current flowops.Record, input flowops.UpdateRequest) (flowops.UpdateResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(input.LUID) == "" || input.OwnerLUID == nil || strings.TrimSpace(*input.OwnerLUID) == "" {
		return flowops.UpdateResult{}, errors.New("flow update requires configured client and exact flow and owner LUIDs")
	}
	result, err := p.changes.Update(ctx, tableauflow.UpdateRequest{LUID: input.LUID, OwnerLUID: input.OwnerLUID})
	output := flowops.UpdateResult{Status: result.Status, FlowLUID: result.FlowLUID, OwnerLUID: result.OwnerLUID, TableauRequestID: result.TableauRequestID}
	if current.LUID == result.FlowLUID {
		output.FlowName, output.ProjectLUID = current.Name, current.ProjectLUID
	}
	return output, err
}

func (p *MutationPort) DeleteFlow(ctx context.Context, flowLUID string) (flowops.DeleteResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(flowLUID) == "" {
		return flowops.DeleteResult{}, errors.New("flow delete requires a configured client and exact flow LUID")
	}
	result, err := p.changes.Delete(ctx, flowLUID)
	return flowops.DeleteResult{Status: result.Status, FlowLUID: result.FlowLUID, TableauRequestID: result.TableauRequestID}, err
}
