package flow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type PullLineageReader interface {
	Capture(context.Context, value.LineageCaptureRequest) (value.LineageCaptureGraph, error)
}

// PullPorts translates native flow, lineage, and local artifact boundaries.
type PullPorts struct {
	Adapter  *Adapter
	Lineage  PullLineageReader
	Manager  *artifact.FlowManager
	Progress func(context.Context, string)
}

func (p PullPorts) mark(ctx context.Context, label string) {
	if p.Progress != nil {
		p.Progress(ctx, label)
	}
}

func (p PullPorts) ResolveFlow(ctx context.Context, selector identity.Selector) (flow.Record, error) {
	return p.Adapter.ResolveFlow(ctx, selector)
}

func (p PullPorts) DownloadFlow(ctx context.Context, luid string) (flow.PullDownload, error) {
	p.mark(ctx, "Downloading flow")
	item, err := p.Adapter.DownloadFlow(ctx, luid)
	return flow.PullDownload{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (p PullPorts) CaptureLineage(ctx context.Context, input flow.PullLineageRequest) (flow.PullLineage, error) {
	p.mark(ctx, "Reading flow metadata")
	graph, err := p.Lineage.Capture(ctx, value.LineageCaptureRequest{Kind: input.Kind, RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	nodes := make([]flow.PullLineageNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		nodes[index] = flow.PullLineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]flow.PullLineageEdge, len(graph.Edges))
	for index, edge := range graph.Edges {
		edges[index] = flow.PullLineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return flow.PullLineage{Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}, err
}

func (p PullPorts) WriteFlow(ctx context.Context, input flow.PullArtifact) (flow.PullArtifactResult, error) {
	p.mark(ctx, "Saving flow files")
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
	result, err := p.Manager.Pull(ctx, artifact.FlowPull{Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite, Metadata: artifact.FlowMetadata{Name: input.Name, TableauID: input.TableauID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID, FileType: input.FileType}, Lineage: artifact.LineageDocument{Complete: input.Lineage.Complete, Direction: direction, Depth: depth, Failure: failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Lineage.Warnings...)}})
	if err != nil {
		return flow.PullArtifactResult{}, err
	}
	canonicalPath, err := containPullWorkspacePath(input.Workspace, result.CanonicalPath, "canonical path")
	if err != nil {
		return flow.PullArtifactResult{}, err
	}
	lineagePath, err := containPullWorkspacePath(input.Workspace, filepath.Join(input.Workspace, filepath.FromSlash(result.LineagePath)), "lineage path")
	if err != nil {
		return flow.PullArtifactResult{}, err
	}
	return flow.PullArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: canonicalPath, BaselineFingerprint: result.BaselineFingerprint, LineagePath: lineagePath, Warnings: append([]string(nil), result.Warnings...)}, nil
}

func containPullWorkspacePath(workspace, absolute, label string) (string, error) {
	relative, err := filepath.Rel(workspace, absolute)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", errors.New("flow artifact " + label + " escapes the resolved workspace")
	}
	return relative, nil
}

func (p PullPorts) PreviewFlow(ctx context.Context, input flow.PullInput, item flow.Record) (value.AcquisitionPlan, error) {
	if item.ProjectLUID == "" || item.ProjectPath == "" {
		return value.AcquisitionPlan{}, &errs.Error{ID: "flow.pull.preview", Kind: errs.KindOperation, Operation: "flow.pull", Environment: input.Environment, Site: input.Site, Summary: "Flow artifact requires complete source project identity.", Retryable: errs.Bool(false), CorrectiveAction: "Resolve the source project before pulling.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	target, err := artifact.PreviewPull(ctx, artifact.PullPreview{Workspace: input.Workspace, Kind: "flow", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, &errs.Error{ID: "flow.pull.preview", Kind: errs.KindOperation, Operation: "flow.pull", Resource: item.LUID, Summary: "Local acquisition preflight failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the local artifact conflict or workspace prerequisite, then retry."}
	}
	return value.AcquisitionPlan{Status: "preview", Operation: "flow.pull", Workspace: input.WorkspaceName, Environment: input.Environment, Site: input.Site, Target: value.AcquisitionTarget{Kind: "flow", LUID: item.LUID, Name: item.Name, Path: target.Path, Exists: target.Exists, Overwrite: input.Overwrite}, Direction: "both", Depth: 1, Limitations: []string{"Execution rechecks local conflicts. Native payload validity, download permission, optional lineage availability, and filesystem write permission are not verified by this preview."}}, nil
}
