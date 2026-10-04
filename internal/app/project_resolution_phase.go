package app

import (
	"context"
)

func (a publishAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.adapter.BeginProjectResolution(ctx)
}
func (a datasourcePublishAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
func (a flowPublishAdapter) BeginProjectResolution(ctx context.Context) context.Context {
	return a.projects.BeginProjectResolution(ctx)
}
