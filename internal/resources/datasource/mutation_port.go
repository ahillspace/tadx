package datasource

import (
	"context"
	"errors"
	"strings"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/value"
)

type DatasourceChanges interface {
	Update(context.Context, tableaudatasource.UpdateRequest) (tableaudatasource.MutationResult, error)
	Delete(context.Context, string) (tableaudatasource.MutationResult, error)
}

type MutationProjectResolver interface {
	ResolveProjectIdentity(context.Context, identity.Selector) (value.ProjectIdentity, error)
	BeginProjectResolution(context.Context) context.Context
}

// MutationPort binds datasource resolution, project resolution, and exact writes.
type MutationPort struct {
	*Adapter
	projects MutationProjectResolver
	changes  DatasourceChanges
}

func NewMutationPort(reader *Adapter, projects MutationProjectResolver, changes DatasourceChanges) *MutationPort {
	return &MutationPort{Adapter: reader, projects: projects, changes: changes}
}

func (p *MutationPort) ResolveProject(ctx context.Context, selector identity.Selector) (datasourceops.Project, error) {
	if p == nil || p.projects == nil {
		return datasourceops.Project{}, errors.New("datasource project resolver is not configured")
	}
	return p.projects.ResolveProjectIdentity(ctx, selector)
}

func (p *MutationPort) BeginProjectResolution(ctx context.Context) context.Context {
	return p.projects.BeginProjectResolution(ctx)
}

func (p *MutationPort) MoveDatasource(ctx context.Context, luid, projectLUID string) (datasourceops.MoveResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(luid) == "" || strings.TrimSpace(projectLUID) == "" {
		return datasourceops.MoveResult{}, errors.New("datasource move requires configured client and exact datasource and project LUIDs")
	}
	result, err := p.changes.Update(ctx, tableaudatasource.UpdateRequest{LUID: luid, ProjectLUID: &projectLUID})
	return datasourceops.MoveResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

func (p *MutationPort) UpdateDatasource(ctx context.Context, input datasourceops.UpdateRequest) (datasourceops.UpdateResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(input.LUID) == "" {
		return datasourceops.UpdateResult{}, errors.New("datasource LUID and configured client are required")
	}
	result, err := p.changes.Update(ctx, tableaudatasource.UpdateRequest{LUID: input.LUID, Name: input.Name, OwnerLUID: input.OwnerLUID})
	return datasourceops.UpdateResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, DatasourceName: result.DatasourceName, ProjectLUID: result.ProjectLUID, OwnerLUID: result.OwnerLUID, TableauRequestID: result.TableauRequestID}, err
}

func (p *MutationPort) DeleteDatasource(ctx context.Context, luid string) (datasourceops.DeleteResult, error) {
	if p == nil || p.changes == nil || strings.TrimSpace(luid) == "" {
		return datasourceops.DeleteResult{}, errors.New("datasource LUID and configured client are required")
	}
	result, err := p.changes.Delete(ctx, luid)
	return datasourceops.DeleteResult{Status: result.Status, DatasourceLUID: result.DatasourceLUID, TableauRequestID: result.TableauRequestID}, err
}
