package app

import (
	"context"
	"github.com/ahillspace/tadx/internal/identity"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
)

// This bridge keeps workbook-owned identity types while sharing the same exact
// hierarchy resolver used for source paths and destination project validation.
type workbookProjectResolver struct{ *resourceproject.Adapter }

func (r workbookProjectResolver) ResolveProject(ctx context.Context, selector identity.Selector) (resourceworkbook.Project, error) {
	item, err := r.Adapter.ResolveProject(ctx, selector)
	return resourceworkbook.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}

func (a workbookMutationAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.workbooks.BeginProjectResolution(ctx)
}
func (a publishAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.adapter.BeginProjectResolution(ctx)
}
func (a datasourceMutationAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
func (a datasourcePublishAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
func (a flowMoveAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
func (a flowPublishAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
func (a projectMoveAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
func (a projectCreateAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
