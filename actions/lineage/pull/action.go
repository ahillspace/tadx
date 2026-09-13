package pull

import "github.com/ahillspace/tadx/internal/value"

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
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
		return Output{}, &errs.Error{ID: "lineage.pull.unconfigured", Kind: errs.KindRuntime, Operation: "lineage.pull", Summary: "Lineage pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure lineage pull before retrying."}
	}
	normalized, err := validateInput(input)
	if err != nil {
		return Output{}, err
	}
	resource, err := a.resolver.ResolveLineageResource(ctx, normalized.Kind, normalized.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact lineage root selector, then retry.")
		return Output{}, &errs.Error{ID: "lineage.pull.resolve", Kind: errs.KindOperation, Operation: "lineage.pull", Environment: normalized.Environment, Site: normalized.Site, Summary: "Lineage root resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if err := validateResolvedResource(normalized, resource); err != nil {
		return Output{}, err
	}
	if normalized.Preview {
		previewer, ok := a.writer.(interface {
			PreviewLineage(context.Context, Input, Resource) (value.AcquisitionPlan, error)
		})
		if !ok {
			return Output{}, &errs.Error{ID: "lineage.pull.preview", Kind: errs.KindRuntime, Operation: "lineage.pull", Summary: "Acquisition preview is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure read-only artifact preflight."}
		}
		plan, err := previewer.PreviewLineage(ctx, normalized, resource)
		if err != nil {
			return Output{}, err
		}
		return Output{Status: "preview", Resource: resource, Preview: &plan}, nil
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
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the workspace and artifact target, then pull again.")
		return Output{}, &errs.Error{ID: "lineage.pull.write", Kind: errs.KindOperation, Operation: "lineage.pull", Resource: resource.LUID, Environment: normalized.Environment, Site: normalized.Site, Summary: "Lineage artifact write failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	return Output{
		Status: "pulled", Resource: resource, Artifact: result, Direction: normalized.Direction, Depth: normalized.Depth,
		Complete: graph.Complete, CountsKnown: countsKnown, Nodes: graph.Nodes, Edges: graph.Edges,
		Warnings: warnings, WarningsOmitted: warningsOmitted,
		Provenance: Provenance{Environment: normalized.Environment, Site: normalized.Site, ServerOrigin: normalized.ServerOrigin, SiteLUID: normalized.SiteLUID},
		RequestIDs: requestIDs, RequestIDsOmitted: requestIDsOmitted,
		Help: []string{commandhint.Target(normalized.Environment, normalized.WorkspaceName, "catalog", "lineage", "pull", "--kind", publicKind(resource.Kind), "--id", resource.LUID, "--direction", normalized.Direction, "--depth", fmt.Sprint(normalized.Depth))},
	}, nil
}

func validateInput(input Input) (Input, error) {
	if strings.TrimSpace(input.Workspace) == "" {
		return Input{}, usage("workspace", "lineage pull requires a workspace")
	}
	return NormalizeInput(input)
}

func validateResolvedResource(input Input, resource Resource) error {
	resource.Kind = strings.TrimSpace(resource.Kind)
	resource.LUID = strings.TrimSpace(resource.LUID)
	resource.Name = strings.TrimSpace(resource.Name)
	if resource.Kind != input.Kind || resource.LUID == "" || resource.Name == "" {
		return &errs.Error{ID: "lineage.pull.resolved_invariant", Kind: errs.KindOperation, Operation: "lineage.pull", Environment: input.Environment, Site: input.Site, Summary: "The resolved lineage root omitted or changed its authoritative identity.", Cause: errors.New("resolved lineage root omitted or changed its authoritative kind, LUID, or name"), Retryable: errs.Bool(false), CorrectiveAction: "Review the exact lineage root selector, then retry."}
	}
	if input.Selector.LUID != "" && resource.LUID != string(input.Selector.LUID) {
		return &errs.Error{ID: "lineage.pull.resolved_invariant", Kind: errs.KindOperation, Operation: "lineage.pull", Environment: input.Environment, Site: input.Site, Summary: "The resolved lineage root does not match the requested LUID.", Cause: errors.New("resolved lineage root does not match the requested authoritative LUID"), Retryable: errs.Bool(false), CorrectiveAction: "Review the exact lineage root LUID, then retry."}
	}
	return nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "lineage.pull.usage", Kind: errs.KindUsage, Operation: "lineage.pull", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the lineage pull input, then retry.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}

func usageCause(field, message string, cause error) error {
	return &errs.Error{ID: "lineage.pull.usage", Kind: errs.KindUsage, Operation: "lineage.pull", Summary: message, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Correct the lineage pull input, then retry.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
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
