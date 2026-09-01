package pull

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

const maxWarnings = 20

// Resolver resolves one authoritative REST resource identity.
type Resolver interface {
	ResolveLineageResource(context.Context, string, identity.Selector) (Resource, error)
}

// Reader captures bounded Metadata lineage for an exact REST LUID.
type Reader interface {
	CaptureLineage(context.Context, CaptureRequest) (Graph, error)
}

// Writer persists one metadata-only lineage artifact.
type Writer interface {
	WriteLineage(context.Context, Artifact) (ArtifactResult, error)
}

// Action pulls one bounded metadata-only lineage graph.
type Action struct {
	resolver Resolver
	reader   Reader
	writer   Writer
}

// New creates a lineage pull action.
func New(resolver Resolver, reader Reader, writer Writer) *Action {
	return &Action{resolver: resolver, reader: reader, writer: writer}
}

// Execute resolves, captures, and persists one metadata-only graph.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil || a.reader == nil || a.writer == nil {
		return Output{}, errors.New("lineage pull dependencies are not configured")
	}
	normalized, err := validateInput(input)
	if err != nil {
		return Output{}, err
	}
	resource, err := a.resolver.ResolveLineageResource(ctx, normalized.Kind, normalized.Selector)
	if err != nil {
		return Output{}, err
	}
	if err := validateResolvedResource(normalized, resource); err != nil {
		return Output{}, err
	}
	request := CaptureRequest{Kind: resource.Kind, RESTLUID: resource.LUID, Direction: normalized.Direction, Depth: normalized.Depth}
	graph, captureErr := a.reader.CaptureLineage(ctx, request)
	countsKnown := captureErr == nil
	if captureErr != nil {
		graph = Graph{Complete: false, Warnings: []string{"Lineage capture was incomplete. Review the selected environment and Metadata API permissions."}}
	}
	resource.MetadataID = strings.TrimSpace(graph.RootMetadataID)
	warnings, warningsOmitted := boundedStrings(graph.Warnings, maxWarnings)
	requestIDs, requestIDsOmitted := boundedStrings(graph.RequestIDs, maxWarnings)
	artifact := Artifact{
		Workspace: normalized.Workspace, Resource: resource, Environment: normalized.Environment, Site: normalized.Site,
		ServerOrigin: normalized.ServerOrigin, SiteLUID: normalized.SiteLUID, Direction: normalized.Direction, Depth: normalized.Depth,
		Complete: graph.Complete, CountsKnown: countsKnown, Nodes: graph.Nodes, Edges: graph.Edges,
		Warnings: warnings, WarningsOmitted: warningsOmitted, RequestIDs: requestIDs, Overwrite: normalized.Overwrite,
	}
	result, err := a.writer.WriteLineage(ctx, artifact)
	if err != nil {
		return Output{}, err
	}
	return Output{
		Status: "pulled", Resource: resource, Artifact: result, Direction: normalized.Direction, Depth: normalized.Depth,
		Complete: graph.Complete, CountsKnown: countsKnown, Nodes: graph.Nodes, Edges: graph.Edges,
		Warnings: warnings, WarningsOmitted: warningsOmitted,
		Provenance: Provenance{Environment: normalized.Environment, Site: normalized.Site, ServerOrigin: normalized.ServerOrigin, SiteLUID: normalized.SiteLUID},
		RequestIDs: requestIDs, RequestIDsOmitted: requestIDsOmitted,
		Help: []string{"tadx content lineage pull --kind " + resource.Kind + " --id " + resource.LUID + " --direction " + normalized.Direction},
	}, nil
}

func validateInput(input Input) (Input, error) {
	input.Kind = strings.TrimSpace(input.Kind)
	input.Direction = strings.TrimSpace(input.Direction)
	if strings.TrimSpace(input.Workspace) == "" {
		return Input{}, errors.New("lineage pull requires a workspace")
	}
	if input.Kind != "workbook" && input.Kind != "published_datasource" && input.Kind != "flow" {
		return Input{}, fmt.Errorf("unsupported lineage root kind %q", input.Kind)
	}
	if input.Selector.LUID == "" && strings.TrimSpace(input.Selector.Name) == "" {
		return Input{}, errors.New("lineage pull requires a REST LUID or exact name selector")
	}
	if input.Direction == "" {
		input.Direction = "both"
	}
	if input.Direction != "upstream" && input.Direction != "downstream" && input.Direction != "both" {
		return Input{}, fmt.Errorf("unsupported lineage direction %q", input.Direction)
	}
	if input.Depth == 0 {
		input.Depth = 1
	}
	if input.Depth < 1 || input.Depth > 3 {
		return Input{}, errors.New("lineage depth must be between 1 and 3")
	}
	return input, nil
}

func validateResolvedResource(input Input, resource Resource) error {
	resource.Kind = strings.TrimSpace(resource.Kind)
	resource.LUID = strings.TrimSpace(resource.LUID)
	resource.Name = strings.TrimSpace(resource.Name)
	if resource.Kind != input.Kind || resource.LUID == "" || resource.Name == "" {
		return errors.New("resolved lineage root omitted or changed its authoritative kind, LUID, or name")
	}
	if input.Selector.LUID != "" && resource.LUID != string(input.Selector.LUID) {
		return errors.New("resolved lineage root does not match the requested authoritative LUID")
	}
	return nil
}

func boundedStrings(values []string, limit int) ([]string, int) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	omitted := 0
	if len(result) > limit {
		omitted = len(result) - limit
		result = result[:limit]
	}
	return result, omitted
}
