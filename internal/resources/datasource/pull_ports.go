package datasource

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type PullLineageReader interface {
	Capture(context.Context, value.LineageCaptureRequest) (value.LineageCaptureGraph, error)
}

// PullPorts translates native datasource, lineage, and artifact boundaries.
type PullPorts struct {
	Adapter  *Adapter
	Lineage  PullLineageReader
	Manager  *artifact.DatasourceManager
	Progress func(context.Context, string)
}

func (p PullPorts) mark(ctx context.Context, label string) {
	if p.Progress != nil {
		p.Progress(ctx, label)
	}
}

func (p PullPorts) ResolveDatasource(ctx context.Context, selector identity.Selector) (datasource.Record, error) {
	return p.Adapter.ResolveDatasource(ctx, selector)
}

func (p PullPorts) DownloadDatasource(ctx context.Context, luid string) (datasource.Download, error) {
	p.mark(ctx, "Downloading datasource")
	item, err := p.Adapter.DownloadDatasource(ctx, luid)
	return datasource.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (p PullPorts) CaptureLineage(ctx context.Context, input datasource.LineageRequest) (datasource.Lineage, error) {
	p.mark(ctx, "Reading datasource metadata")
	graph, err := p.Lineage.Capture(ctx, value.LineageCaptureRequest{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	nodes := make([]datasource.LineageNode, len(graph.Nodes))
	copy(nodes, graph.Nodes)
	edges := make([]datasource.LineageEdge, len(graph.Edges))
	copy(edges, graph.Edges)
	return datasource.Lineage{Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}, err
}

func (p PullPorts) WriteDatasource(ctx context.Context, input datasource.PullArtifact) (datasource.PullArtifactResult, error) {
	p.mark(ctx, "Saving datasource files")
	nodes := make([]artifact.LineageNode, len(input.Lineage.Nodes))
	for index, node := range input.Lineage.Nodes {
		nodes[index] = artifact.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]artifact.LineageEdge, len(input.Lineage.Edges))
	for index, edge := range input.Lineage.Edges {
		edges[index] = artifact.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	direction, depth := input.Lineage.Direction, input.Lineage.Depth
	if direction == "" {
		direction = "both"
	}
	if depth == 0 {
		depth = 1
	}
	var failure *artifact.LineageFailure
	if input.Lineage.Failure != nil {
		item := input.Lineage.Failure
		failure = &artifact.LineageFailure{Provider: item.Provider, Relation: item.Relation, RootKind: item.RootKind, RootRESTLUID: item.RootRESTLUID, RequestID: item.RequestID}
	}
	result, err := p.Manager.Pull(ctx, artifact.DatasourcePull{
		Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite,
		Metadata: artifact.DatasourceMetadata{Name: input.Name, TableauID: input.TableauID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID},
		Lineage:  artifact.LineageDocument{Complete: input.Lineage.Complete, Direction: direction, Depth: depth, Failure: failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Lineage.Warnings...)},
	})
	if err != nil {
		return datasource.PullArtifactResult{}, err
	}
	canonicalPath, err := containDatasourceWorkspacePath(input.Workspace, result.CanonicalPath, "canonical path")
	if err != nil {
		return datasource.PullArtifactResult{}, err
	}
	lineagePath, err := containDatasourceWorkspacePath(input.Workspace, filepath.Join(input.Workspace, filepath.FromSlash(result.LineagePath)), "lineage path")
	if err != nil {
		return datasource.PullArtifactResult{}, err
	}
	return datasource.PullArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: canonicalPath, BaselineFingerprint: result.BaselineFingerprint, LineagePath: lineagePath, CompositionStatus: result.CompositionStatus, ParentDataSourceURLs: append([]string(nil), result.ParentDataSourceURLs...), Warnings: append([]string(nil), result.Warnings...)}, nil
}

func containDatasourceWorkspacePath(workspace, absolute, label string) (string, error) {
	relative, err := filepath.Rel(workspace, absolute)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", errors.New("datasource artifact " + label + " escapes the resolved workspace")
	}
	return relative, nil
}

func (p PullPorts) PreviewDatasource(ctx context.Context, input datasource.PullInput, item datasource.Record) (value.AcquisitionPlan, error) {
	target, err := artifact.PreviewPull(ctx, artifact.PullPreview{Workspace: input.Workspace, Kind: "datasource", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, &errs.Error{ID: "datasource.pull.preview", Kind: errs.KindOperation, Operation: "datasource.pull", Resource: item.LUID, Summary: "Local acquisition preflight failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the local artifact conflict or workspace prerequisite, then retry."}
	}
	return value.AcquisitionPlan{Status: "preview", Operation: "datasource.pull", Workspace: input.WorkspaceName, Environment: input.Environment, Site: input.Site, Target: value.AcquisitionTarget{Kind: "datasource", LUID: item.LUID, Name: item.Name, Path: target.Path, Exists: target.Exists, Overwrite: input.Overwrite}, Direction: "both", Depth: 1, Limitations: []string{"Execution rechecks local conflicts. Native payload validity, download permission, optional lineage availability, and filesystem write permission are not verified by this preview."}}, nil
}
