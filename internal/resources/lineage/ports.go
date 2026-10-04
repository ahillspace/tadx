package lineage

import (
	"context"
	"errors"

	lineageops "github.com/ahillspace/tadx/actions/lineage"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// WorkbookResolver supplies an exact native workbook identity.
type WorkbookResolver interface {
	ResolveWorkbook(context.Context, identity.Selector) (value.Workbook, error)
}

// DatasourceResolver supplies an exact native datasource identity.
type DatasourceResolver interface {
	ResolveDatasource(context.Context, identity.Selector) (value.Datasource, error)
}

// FlowResolver supplies an exact native flow identity.
type FlowResolver interface {
	ResolveFlow(context.Context, identity.Selector) (value.Flow, error)
}

// ReadPorts implements lineage's typed source and artifact contracts.
type ReadPorts struct {
	Workbooks   WorkbookResolver
	Datasources DatasourceResolver
	Flows       FlowResolver
	Metadata    *Adapter
	Artifacts   *artifact.LineageManager
}

// ResolveLineageResource maps the selected resource's canonical REST identity.
func (p ReadPorts) ResolveLineageResource(ctx context.Context, kind string, selector identity.Selector) (lineageops.Resource, error) {
	switch kind {
	case "workbook":
		item, err := p.Workbooks.ResolveWorkbook(ctx, selector)
		return lineageops.Resource{Kind: kind, LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath}, err
	case "flow":
		item, err := p.Flows.ResolveFlow(ctx, selector)
		return lineageops.Resource{Kind: kind, LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath}, err
	case "published_datasource":
		item, err := p.Datasources.ResolveDatasource(ctx, selector)
		if err != nil {
			return lineageops.Resource{}, err
		}
		return lineageops.Resource{Kind: kind, LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath}, nil
	default:
		return lineageops.Resource{}, errors.New("unsupported lineage resource kind")
	}
}

// CaptureLineage projects one normalized Metadata graph without dropping partial evidence.
func (p ReadPorts) CaptureLineage(ctx context.Context, input lineageops.CaptureRequest) (lineageops.Graph, error) {
	graph, err := p.Metadata.Capture(ctx, Request{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	return lineageops.Graph{RootMetadataID: graph.RootMetadataID, Direction: graph.Direction, Depth: graph.Depth, Complete: graph.Complete, Failure: graph.Failure, Nodes: graph.Nodes, Edges: graph.Edges, Warnings: graph.Warnings, RequestIDs: graph.RequestIDs}, err
}

// WriteLineage persists the bounded graph as a metadata-only artifact.
func (p ReadPorts) WriteLineage(ctx context.Context, input lineageops.Artifact) (lineageops.ArtifactResult, error) {
	nodes := make([]artifact.LineageNode, len(input.Nodes))
	for index, node := range input.Nodes {
		nodes[index] = artifact.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]artifact.LineageEdge, len(input.Edges))
	for index, edge := range input.Edges {
		edges[index] = artifact.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	result, err := p.Artifacts.Pull(ctx, artifact.LineagePull{Workspace: input.Workspace, CountsKnown: input.CountsKnown, Overwrite: input.Overwrite, Metadata: artifact.LineageMetadata{ResourceKind: input.Resource.Kind, Name: input.Resource.Name, TableauID: input.Resource.LUID, MetadataID: input.Resource.MetadataID, ProjectPath: input.Resource.ProjectPath, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site}, Lineage: artifact.LineageDocument{Complete: input.Complete, Direction: input.Direction, Depth: input.Depth, Failure: lineageFailure(input.Failure), Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Warnings...)}})
	return lineageops.ArtifactResult{Path: result.Path, LineagePath: result.LineagePath, Fingerprint: result.Fingerprint}, err
}

func lineageFailure(failure *value.LineageFailure) *artifact.LineageFailure {
	if failure == nil {
		return nil
	}
	return &artifact.LineageFailure{Provider: failure.Provider, Relation: failure.Relation, RootKind: failure.RootKind, RootRESTLUID: failure.RootRESTLUID, RequestID: failure.RequestID}
}

// PreviewLineage checks only the rooted local target; it does not capture Metadata.
func (p ReadPorts) PreviewLineage(ctx context.Context, input lineageops.Input, item lineageops.Resource) (value.AcquisitionPlan, error) {
	target, err := artifact.PreviewPull(ctx, artifact.PullPreview{Workspace: input.Workspace, Kind: "lineage", ResourceKind: item.Kind, Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, &errs.Error{ID: "lineage.pull.preview", Kind: errs.KindOperation, Operation: "lineage.pull", Resource: item.LUID, Summary: "Local acquisition preflight failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the local artifact conflict or workspace prerequisite, then retry."}
	}
	plan := value.AcquisitionPlan{Status: "preview", Operation: "lineage.pull", Workspace: input.WorkspaceName, Environment: input.Environment, Site: input.Site, Target: value.AcquisitionTarget{Kind: "lineage", ResourceKind: item.Kind, LUID: item.LUID, Name: item.Name, Path: target.Path, Exists: target.Exists, Overwrite: input.Overwrite}}
	plan.Direction, plan.Depth = input.Direction, input.Depth
	plan.Limitations = []string{"The graph is not captured during preview. Metadata API availability, completeness, and filesystem write permission are checked during execution."}
	return plan, nil
}
