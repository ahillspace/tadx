// Package datasource implements explicit published datasource operations.
package datasource

import (
	"context"

	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type Record = value.Datasource
type Project = value.ProjectIdentity

type Resolver interface {
	ResolveDatasource(context.Context, identity.Selector) (Record, error)
}

type CollisionReader interface {
	FindDatasources(context.Context, string, string) ([]Record, error)
}

type ProjectResolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
}

func moveIdentity(item Record) moveDatasource {
	return moveDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
}

func updateIdentity(item Record) updateDatasource {
	return updateDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath, OwnerLUID: item.OwnerLUID}
}

func deleteIdentity(item Record) deleteDatasource {
	return deleteDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}
}

func pullIdentity(item Record) pullDatasource {
	return pullDatasource{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}
}

func inspectRecord(item Record) InspectDatasource {
	return InspectDatasource{
		Upstream: item.Upstream, LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath,
		Type: item.Type, ContentURL: item.ContentURL, Description: item.Description, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement, RequestID: item.RequestID,
	}
}

func listRecord(item Record) listDatasource {
	return listDatasource{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: item.ProjectPath,
		Type: item.Type, ContentURL: item.ContentURL, Description: item.Description, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement,
	}
}

func beginProjectResolution(ctx context.Context, resolver any) context.Context {
	if phased, ok := resolver.(interface {
		BeginProjectResolution(context.Context) context.Context
	}); ok {
		return phased.BeginProjectResolution(ctx)
	}
	return ctx
}
