package workbook

import (
	"context"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/value"
)

type PullWriterPort struct {
	Workbooks *artifact.WorkbookManager
	Bundles   *artifact.WorkbookBundleManager
	Progress  func(context.Context, string)
}

func (w PullWriterPort) mark(ctx context.Context, label string) {
	if w.Progress != nil {
		w.Progress(ctx, label)
	}
}

func (w PullWriterPort) WriteWorkbook(ctx context.Context, input workbookops.PullArtifact) (workbookops.PullArtifactResult, error) {
	w.mark(ctx, "Saving workbook files")
	references := make([]artifact.PublishedDatasourceRef, len(input.PublishedDatasources))
	for index, item := range input.PublishedDatasources {
		references[index] = artifact.PublishedDatasourceRef{LUID: item.LUID, Name: item.Name, SourceSite: item.SourceSite, LocalArtifactPath: item.LocalArtifactPath}
	}
	result, err := w.Workbooks.Pull(ctx, artifact.WorkbookPull{Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite, Lineage: workbookLineageDocument(input.Lineage), LineageCountsKnown: input.LineageCountsKnown, Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: input.Name, TableauID: input.TableauID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID, Portability: input.Portability, PublishedDatasources: references, DependenciesAcquired: input.DependenciesAcquired}})
	return workbookops.PullArtifactResult{Path: result.ArtifactPath, CanonicalPath: result.CanonicalPath, LineagePath: result.LineagePath, LineageStatus: result.LineageStatus, BaselineFingerprint: result.BaselineFingerprint, Warnings: result.Warnings}, err
}

func (w PullWriterPort) WriteBundle(ctx context.Context, workbook workbookops.PullArtifact, datasources []workbookops.DatasourceArtifact) (workbookops.PullArtifactResult, error) {
	w.mark(ctx, "Saving workbook and datasource files")
	references := make([]artifact.PublishedDatasourceRef, len(workbook.PublishedDatasources))
	for index, item := range workbook.PublishedDatasources {
		references[index] = artifact.PublishedDatasourceRef{LUID: item.LUID, Name: item.Name, SourceSite: item.SourceSite}
	}
	bundle := artifact.WorkbookBundlePull{
		Workbook:    artifact.WorkbookPull{Workspace: workbook.Workspace, Filename: workbook.Filename, Content: workbook.Content, Overwrite: workbook.Overwrite, Lineage: workbookLineageDocument(workbook.Lineage), LineageCountsKnown: workbook.LineageCountsKnown, Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: workbook.Name, TableauID: workbook.TableauID, SourceServerOrigin: workbook.ServerOrigin, SourceSiteLUID: workbook.SiteLUID, SourceEnvironment: workbook.Environment, SourceSite: workbook.Site, SourceProjectName: workbook.ProjectName, SourceProjectID: workbook.ProjectID, Portability: workbook.Portability, PublishedDatasources: references, DependenciesAcquired: true}},
		Datasources: make([]artifact.DatasourcePull, len(datasources)),
	}
	for index, item := range datasources {
		bundle.Datasources[index] = artifact.DatasourcePull{Workspace: item.Workspace, Filename: item.Filename, Content: item.Content, Overwrite: item.Overwrite, Metadata: artifact.DatasourceMetadata{Kind: "datasource", Name: item.Name, TableauID: item.TableauID, SourceServerOrigin: item.ServerOrigin, SourceSiteLUID: item.SiteLUID, SourceEnvironment: item.Environment, SourceSite: item.Site, SourceProjectName: item.ProjectName, SourceProjectID: item.ProjectID}}
	}
	result, err := w.Bundles.Pull(ctx, bundle)
	if err != nil {
		return workbookops.PullArtifactResult{}, err
	}
	dependencies := make([]workbookops.DependencyArtifactResult, len(result.Datasources))
	pathByLUID := make(map[string]string, len(result.Datasources))
	for index, item := range result.Datasources {
		source := datasources[index]
		dependencies[index] = workbookops.DependencyArtifactResult{LUID: source.TableauID, Name: source.Name, Path: item.WorkspaceRelativePath, CanonicalPath: item.CanonicalPath, BaselineFingerprint: item.BaselineFingerprint, Warnings: item.Warnings}
		pathByLUID[source.TableauID] = item.WorkspaceRelativePath
	}
	outputReferences := make([]workbookops.PublishedDatasourceRef, len(workbook.PublishedDatasources))
	for index, item := range workbook.PublishedDatasources {
		item.LocalArtifactPath = pathByLUID[item.LUID]
		outputReferences[index] = item
	}
	return workbookops.PullArtifactResult{Path: result.Workbook.ArtifactPath, CanonicalPath: result.Workbook.CanonicalPath, LineagePath: result.Workbook.LineagePath, LineageStatus: result.Workbook.LineageStatus, BaselineFingerprint: result.Workbook.BaselineFingerprint, Portability: workbook.Portability, PublishedDatasources: outputReferences, DependenciesAcquired: true, Dependencies: dependencies, Warnings: result.Workbook.Warnings}, nil
}

func workbookLineageDocument(input workbookops.LineageCapture) artifact.LineageDocument {
	nodes := make([]artifact.LineageNode, len(input.Nodes))
	for index, node := range input.Nodes {
		nodes[index] = artifact.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]artifact.LineageEdge, len(input.Edges))
	for index, edge := range input.Edges {
		edges[index] = artifact.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return artifact.LineageDocument{Complete: input.Complete, Direction: input.Direction, Depth: input.Depth, Failure: artifactLineageFailure(input.Failure), Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Warnings...)}
}

func artifactLineageFailure(failure *value.LineageFailure) *artifact.LineageFailure {
	if failure == nil {
		return nil
	}
	return &artifact.LineageFailure{Provider: failure.Provider, Relation: failure.Relation, RootKind: failure.RootKind, RootRESTLUID: failure.RootRESTLUID, RequestID: failure.RequestID}
}
