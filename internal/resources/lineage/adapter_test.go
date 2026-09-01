package lineage_test

import (
	"context"
	"strings"
	"testing"

	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	tableaumetadata "github.com/ahillspace/tadx/internal/tableau/metadata"
)

type captureClient struct {
	result tableaumetadata.Capture
	input  tableaumetadata.CaptureRequest
}

func (c *captureClient) CaptureLineage(_ context.Context, input tableaumetadata.CaptureRequest) (tableaumetadata.Capture, error) {
	c.input = input
	return c.result, nil
}

func TestAdapterNormalizesAndSortsLineageDeterministically(t *testing.T) {
	client := &captureClient{result: tableaumetadata.Capture{
		RootRESTLUID: "flow-1", RootMetadataID: "meta-flow", Complete: true,
		Nodes: []tableaumetadata.Node{
			{MetadataID: " z ", Kind: " flow ", RESTLUID: " flow-2 ", Name: " Later "},
			{MetadataID: "meta-flow", Kind: "flow", RESTLUID: "flow-1", Name: "Daily"},
		},
		Edges:    []tableaumetadata.Edge{{FromMetadataID: "meta-flow", ToMetadataID: " z ", Relationship: " downstream "}},
		Warnings: []string{" warning-b ", "warning-a", "warning-a"}, RequestIDs: []string{"request-2", "request-1"},
	}}
	graph, err := resourcelineage.NewAdapter(client).Capture(context.Background(), resourcelineage.Request{Kind: "flow", RESTLUID: "flow-1", Direction: "both", Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if graph.RootMetadataID != "meta-flow" || len(graph.Nodes) != 2 || graph.Nodes[0].MetadataID != "meta-flow" || graph.Nodes[1].MetadataID != "z" {
		t.Fatalf("graph = %#v", graph)
	}
	if len(graph.Warnings) != 2 || graph.Warnings[0] != "warning-a" || graph.Edges[0].ToMetadataID != "z" {
		t.Fatalf("normalized graph = %#v", graph)
	}
	if client.input.Kind != tableaumetadata.KindFlow || client.input.RESTLUID != "flow-1" {
		t.Fatalf("input = %#v", client.input)
	}
}

func TestAdapterTruncatesGraphAndMarksItIncomplete(t *testing.T) {
	nodes := make([]tableaumetadata.Node, resourcelineage.MaxNodes+1)
	for index := range nodes {
		nodes[index] = tableaumetadata.Node{MetadataID: strings.Repeat("x", 4) + formatIndex(index), Kind: "flow"}
	}
	nodes[0].RESTLUID = "flow-1"
	client := &captureClient{result: tableaumetadata.Capture{RootRESTLUID: "flow-1", RootMetadataID: nodes[0].MetadataID, Complete: true, Nodes: nodes}}
	graph, err := resourcelineage.NewAdapter(client).Capture(context.Background(), resourcelineage.Request{Kind: "flow", RESTLUID: "flow-1"})
	if err != nil {
		t.Fatal(err)
	}
	if graph.Complete || len(graph.Nodes) != resourcelineage.MaxNodes || len(graph.Warnings) == 0 {
		t.Fatalf("graph = %#v", graph)
	}
}

func TestAdapterRejectsConflictsAndUnknownEdgeEndpoints(t *testing.T) {
	tests := []tableaumetadata.Capture{
		{RootRESTLUID: "flow-1", RootMetadataID: "root", Nodes: []tableaumetadata.Node{{MetadataID: "root", Kind: "flow", RESTLUID: "flow-1"}, {MetadataID: "root", Kind: "flow", RESTLUID: "flow-2"}}},
		{RootRESTLUID: "flow-1", RootMetadataID: "root", Complete: true, Nodes: []tableaumetadata.Node{{MetadataID: "root", Kind: "workbook", RESTLUID: "flow-1"}}},
		{RootRESTLUID: "flow-1", RootMetadataID: "root", Nodes: []tableaumetadata.Node{{MetadataID: "root", Kind: "flow", RESTLUID: "flow-1"}}, Edges: []tableaumetadata.Edge{{FromMetadataID: "root", ToMetadataID: "missing", Relationship: "downstream"}}},
	}
	for _, capture := range tests {
		if _, err := resourcelineage.NewAdapter(&captureClient{result: capture}).Capture(context.Background(), resourcelineage.Request{Kind: "flow", RESTLUID: "flow-1"}); err == nil {
			t.Fatalf("expected error for %#v", capture)
		}
	}
}

func formatIndex(index int) string {
	const digits = "0123456789"
	return string([]byte{digits[(index/100)%10], digits[(index/10)%10], digits[index%10]})
}
