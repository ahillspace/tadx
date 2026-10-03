package project

import (
	"context"
	"strings"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/identity"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

// MutationClient is the native project mutation boundary.
type MutationClient interface {
	Create(context.Context, tableauproject.CreateRequest) (tableauproject.MutationResult, error)
	Update(context.Context, tableauproject.UpdateRequest) (tableauproject.MutationResult, error)
	Delete(context.Context, string) (tableauproject.DeleteResult, error)
}

var (
	_ projectops.ListReader      = ListPort{}
	_ projectops.InspectResolver = InspectPort{}
	_ projectops.CreateResolver  = CreatePort{}
	_ projectops.CreateCreator   = CreatePort{}
	_ projectops.UpdateResolver  = UpdatePort{}
	_ projectops.Updater         = UpdatePort{}
	_ projectops.DeleteResolver  = DeletePort{}
	_ projectops.Deleter         = DeletePort{}
	_ projectops.MoveResolver    = MovePort{}
	_ projectops.Mover           = MovePort{}
)

// ListPort translates one bounded native page into the project action contract.
type ListPort struct{ *Adapter }

// ListFilter validates and encodes the native project list selection.
func ListFilter(input projectops.ListInput) (string, error) {
	return tableauproject.ListFilter(tableauproject.ListRequest{Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel})
}

func (p ListPort) ListProjects(ctx context.Context, input projectops.ListPageRequest) (projectops.ListPage, error) {
	page, err := p.Adapter.ListProjects(ctx, ListRequest{PageNumber: input.PageNumber, PageSize: input.PageSize, Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel})
	items := make([]projectops.ListProject, len(page.Items))
	for index, item := range page.Items {
		items[index] = projectops.ListProject{LUID: item.LUID, Name: item.Name, ParentLUID: item.ParentLUID, Description: item.Description, OwnerLUID: item.OwnerLUID, TopLevel: item.TopLevel, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ProjectCount: item.ProjectCount, WorkbookCount: item.WorkbookCount, ViewCount: item.ViewCount, DatasourceCount: item.DatasourceCount}
	}
	return projectops.ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Projects: items, RequestID: page.RequestID}, err
}

// InspectPort supplies one exact, normalized project to the inspect action.
type InspectPort struct{ *Adapter }

func (p InspectPort) ResolveProject(ctx context.Context, selector identity.Selector) (projectops.InspectProject, error) {
	item, err := p.Adapter.ResolveProject(ctx, selector)
	return projectops.InspectProject{LUID: item.LUID, Name: item.Name, Path: item.Path, ParentLUID: item.ParentLUID, Description: item.Description, OwnerLUID: item.OwnerLUID, TopLevel: item.TopLevel, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ProjectCount: item.ProjectCount, WorkbookCount: item.WorkbookCount, ViewCount: item.ViewCount, DatasourceCount: item.DatasourceCount, RequestID: item.RequestID}, err
}

type mutationPort struct {
	projects *Adapter
	changes  MutationClient
	resolved map[string]Project
}

func newMutationPort(projects *Adapter, changes MutationClient) mutationPort {
	return mutationPort{projects: projects, changes: changes, resolved: make(map[string]Project)}
}

func (p mutationPort) resolve(ctx context.Context, selector identity.Selector) (Project, error) {
	item, err := p.projects.ResolveProject(ctx, selector)
	if err == nil {
		p.resolved[item.LUID] = item
	}
	return item, err
}

func (p mutationPort) collisions(ctx context.Context, name, parentLUID string) ([]Project, error) {
	return p.projects.FindProjectCollisions(ctx, name, parentLUID)
}

func (p mutationPort) BeginProjectResolution(ctx context.Context) context.Context {
	return p.projects.BeginProjectResolution(ctx)
}

// CreatePort owns native request translation and accepted-result preservation.
type CreatePort struct{ mutationPort }

func NewCreatePort(projects *Adapter, changes MutationClient) CreatePort {
	return CreatePort{newMutationPort(projects, changes)}
}

func (p CreatePort) ResolveProject(ctx context.Context, selector identity.Selector) (projectops.CreateProject, error) {
	item, err := p.resolve(ctx, selector)
	return mutationProject(item), err
}

func (p CreatePort) FindProjectCollisions(ctx context.Context, name, parentLUID string) ([]projectops.CreateProject, error) {
	items, err := p.collisions(ctx, name, parentLUID)
	result := make([]projectops.CreateProject, len(items))
	for index, item := range items {
		result[index] = mutationProject(item)
	}
	return result, err
}

func (p CreatePort) CreateProject(ctx context.Context, input projectops.CreateRequest) (projectops.CreateResult, error) {
	result, err := p.changes.Create(ctx, tableauproject.CreateRequest{Name: input.Name, Description: input.Description, ParentLUID: input.ParentLUID, ContentPermissions: input.ContentPermissions})
	if err != nil {
		item := result.Project
		return projectops.CreateResult{Status: result.Status, Project: projectops.CreateProject{LUID: item.LUID, Name: item.Name, ParentLUID: item.ParentLUID, Description: item.Description, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}, TableauRequestID: result.TableauRequestID}, err
	}
	item := normalizeSuccessfulMutation(ctx, p.projects, p.resolved, result.Project)
	return projectops.CreateResult{Status: result.Status, Project: mutationProject(item), TableauRequestID: result.TableauRequestID}, nil
}

func mutationProject(item Project) projectops.MutationProject {
	return projectops.MutationProject{LUID: item.LUID, Name: item.Name, Path: item.Path, ParentLUID: item.ParentLUID, Description: item.Description, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}
}

// UpdatePort owns native metadata mutation translation.
type UpdatePort struct{ mutationPort }

func NewUpdatePort(projects *Adapter, changes MutationClient) UpdatePort {
	return UpdatePort{newMutationPort(projects, changes)}
}

func (p UpdatePort) ResolveProject(ctx context.Context, selector identity.Selector) (projectops.UpdateProject, error) {
	item, err := p.resolve(ctx, selector)
	return mutationProject(item), err
}

func (p UpdatePort) UpdateProject(ctx context.Context, input projectops.UpdateRequest) (projectops.UpdateResult, error) {
	result, err := p.changes.Update(ctx, tableauproject.UpdateRequest{LUID: input.LUID, Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions})
	if err != nil {
		return projectops.UpdateResult{}, err
	}
	item := normalizeSuccessfulMutation(ctx, p.projects, p.resolved, result.Project)
	return projectops.UpdateResult{Status: result.Status, Project: mutationProject(item), TableauRequestID: result.TableauRequestID}, nil
}

// DeletePort owns the exact native deletion boundary.
type DeletePort struct{ mutationPort }

func NewDeletePort(projects *Adapter, changes MutationClient) DeletePort {
	return DeletePort{newMutationPort(projects, changes)}
}

func (p DeletePort) ResolveProject(ctx context.Context, selector identity.Selector) (projectops.DeleteProject, error) {
	item, err := p.resolve(ctx, selector)
	return projectops.DeleteProject{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (p DeletePort) DeleteProject(ctx context.Context, luid string) (projectops.DeleteResult, error) {
	result, err := p.changes.Delete(ctx, luid)
	return projectops.DeleteResult{Status: result.Status, ProjectLUID: result.ProjectLUID, TableauRequestID: result.TableauRequestID}, err
}

// MovePort owns exact hierarchy resolution and native parent update translation.
type MovePort struct{ mutationPort }

func NewMovePort(projects *Adapter, changes MutationClient) MovePort {
	return MovePort{newMutationPort(projects, changes)}
}

func (p MovePort) ResolveProject(ctx context.Context, selector identity.Selector) (projectops.MoveProject, error) {
	item, err := p.resolve(ctx, selector)
	return moveProject(item), err
}

func (p MovePort) FindProjectCollisions(ctx context.Context, name, parentLUID string) ([]projectops.MoveProject, error) {
	items, err := p.collisions(ctx, name, parentLUID)
	result := make([]projectops.MoveProject, len(items))
	for index, item := range items {
		result[index] = moveProject(item)
	}
	return result, err
}

func (p MovePort) MoveProject(ctx context.Context, luid string, parentLUID *string) (projectops.MoveResult, error) {
	result, err := p.changes.Update(ctx, tableauproject.UpdateRequest{LUID: luid, ParentLUID: parentLUID})
	if err != nil {
		return projectops.MoveResult{}, err
	}
	item := normalizeSuccessfulMutation(ctx, p.projects, p.resolved, result.Project)
	return projectops.MoveResult{Status: result.Status, Project: moveProject(item), TableauRequestID: result.TableauRequestID}, nil
}

func moveProject(item Project) projectops.MoveProject {
	return projectops.MoveProject{LUID: item.LUID, Name: item.Name, Path: item.Path, PathUnavailableReason: item.PathUnavailableReason, ParentLUID: item.ParentLUID, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}
}

const LiteralSlashPathUnavailable = "project name contains a literal slash; canonical hierarchy path is unavailable"

func normalizeSuccessfulMutation(ctx context.Context, projects *Adapter, resolved map[string]Project, item tableauproject.Project) Project {
	result := Project{LUID: item.LUID, Name: item.Name, ParentLUID: item.ParentLUID, Description: item.Description, ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID}
	if item.Name == "" {
		return result
	}
	if strings.Contains(item.Name, "/") {
		result.PathUnavailableReason = LiteralSlashPathUnavailable
		return result
	}
	if item.ParentLUID == "" {
		result.Path = item.Name
		return result
	}
	if parent, ok := resolved[item.ParentLUID]; ok && parent.Path != "" {
		result.Path = parent.Path + "/" + item.Name
		return result
	}
	if source, ok := resolved[item.LUID]; ok && source.ParentLUID == item.ParentLUID && strings.HasSuffix(source.Path, "/"+source.Name) {
		result.Path = strings.TrimSuffix(source.Path, source.Name) + item.Name
		return result
	}
	if projects != nil {
		if enriched, err := projects.NormalizeMutationProject(ctx, item); err == nil {
			return enriched
		}
	}
	return result
}
