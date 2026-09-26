// Package flow implements explicit Tableau flow lifecycle operations.
package flow

import (
	"context"

	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// Record is an internal normalized flow, never a direct output projection.
type Record = value.Flow

// Project is the authoritative destination shared by move and publish.
type Project = value.ProjectIdentity

type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Record, error)
}

type CollisionReader interface {
	FindFlows(context.Context, string, string) ([]Record, error)
}

type ProjectResolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

func contentIdentity(item Record) value.ContentIdentity {
	return value.ContentIdentity{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}
}

func updateIdentity(item Record) UpdateFlow {
	return UpdateFlow{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
}

func beginProjectResolution(ctx context.Context, resolver any) context.Context {
	if phased, ok := resolver.(interface {
		BeginProjectResolution(context.Context) context.Context
	}); ok {
		return phased.BeginProjectResolution(ctx)
	}
	return ctx
}
