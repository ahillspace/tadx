// Package workbook implements explicit workbook lifecycle operations.
package workbook

import (
	"context"

	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// Record is an internal normalized workbook, never an output projection.
type Record = value.Workbook

// Project is the authoritative destination shared by move and publish.
type Project = value.ProjectIdentity

// Resolver reads an authoritative workbook at the point of use.
type Resolver interface {
	ResolveWorkbook(context.Context, identity.Selector) (Record, error)
}

// CollisionReader reads exact name collisions in one project.
type CollisionReader interface {
	FindWorkbooks(context.Context, string, string) ([]Record, error)
}

// ProjectResolver resolves the exact destination project.
type ProjectResolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

func moveIdentity(item Record) moveWorkbook {
	return moveWorkbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
}

func updateIdentity(item Record) updateWorkbook {
	return updateWorkbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID, Description: item.Description}
}

func deleteIdentity(item Record) deleteWorkbook {
	return deleteWorkbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}
}

// beginProjectResolution deliberately starts a fresh observation phase at each
// call site; planning, prewrite, and post-upload phases cannot share snapshots.
func beginProjectResolution(ctx context.Context, resolver any) context.Context {
	if phased, ok := resolver.(interface {
		BeginProjectResolution(context.Context) context.Context
	}); ok {
		return phased.BeginProjectResolution(ctx)
	}
	return ctx
}
