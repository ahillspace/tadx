package workbook

import (
	"context"
	"fmt"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type PullLineageReader interface {
	Capture(context.Context, value.LineageCaptureRequest) (value.LineageCaptureGraph, error)
}

type DatasourceDependencyDownloader interface {
	DownloadDatasource(context.Context, string) (value.DatasourceNativeDownload, error)
}

// PullReaderPort translates native workbook, dependency, and lineage reads.
type PullReaderPort struct {
	Workbooks   *Adapter
	References  *ReferenceAdapter
	Datasources DatasourceDependencyDownloader
	Lineage     PullLineageReader
	Progress    func(context.Context, string)
}

func (p PullReaderPort) mark(ctx context.Context, label string) {
	if p.Progress != nil {
		p.Progress(ctx, label)
	}
}

func (p PullReaderPort) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbook.Record, error) {
	p.mark(ctx, "Resolving workbook")
	return p.Workbooks.ResolveWorkbook(ctx, selector)
}

func (p PullReaderPort) DownloadWorkbook(ctx context.Context, luid string, include *bool) (workbook.Download, error) {
	p.mark(ctx, "Downloading workbook")
	item, err := p.Workbooks.DownloadWorkbook(ctx, luid, include)
	return workbook.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (p PullReaderPort) PublishedDatasources(ctx context.Context, luid string) ([]workbook.PublishedDatasource, error) {
	p.mark(ctx, "Reading workbook dependencies")
	items, err := p.References.PublishedDatasources(ctx, luid)
	result := make([]workbook.PublishedDatasource, len(items))
	for index, item := range items {
		result[index] = workbook.PublishedDatasource{LUID: item.LUID, Name: item.Name}
	}
	return result, err
}

func (p PullReaderPort) DownloadPublishedDatasource(ctx context.Context, luid string) (workbook.DatasourceDownload, error) {
	p.mark(ctx, "Downloading workbook datasource dependency")
	item, err := p.Datasources.DownloadDatasource(ctx, luid)
	if err != nil {
		return workbook.DatasourceDownload{}, err
	}
	project, err := p.Workbooks.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(item.ProjectLUID)})
	if err != nil {
		return workbook.DatasourceDownload{}, fmt.Errorf("resolve datasource project %q: %w", item.ProjectLUID, err)
	}
	return workbook.DatasourceDownload{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: project.Path, Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, nil
}

func (p PullReaderPort) CaptureWorkbookLineage(ctx context.Context, input workbook.LineageRequest) (workbook.LineageCapture, error) {
	p.mark(ctx, "Reading workbook metadata")
	graph, err := p.Lineage.Capture(ctx, value.LineageCaptureRequest{Kind: "workbook", RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	nodes := make([]workbook.LineageNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		nodes[index] = workbook.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]workbook.LineageEdge, len(graph.Edges))
	for index, edge := range graph.Edges {
		edges[index] = workbook.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return workbook.LineageCapture{RootMetadataID: graph.RootMetadataID, Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}, err
}
