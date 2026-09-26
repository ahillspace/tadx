package flow

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pathspec"
	"github.com/ahillspace/tadx/internal/value"
)

type PullReader interface {
	Resolver
	DownloadFlow(context.Context, string) (PullDownload, error)
	CaptureLineage(context.Context, PullLineageRequest) (PullLineage, error)
}
type PullWriter interface {
	WriteFlow(context.Context, PullArtifact) (PullArtifactResult, error)
}

// Pull acquires one exact native flow artifact.
func Pull(ctx context.Context, reader PullReader, writer PullWriter, input PullInput) (PullOutput, error) {
	if err := ValidatePullInput(input); err != nil {
		return PullOutput{}, err
	}
	if reader == nil || writer == nil {
		return PullOutput{}, &errs.Error{ID: "flow.pull.unconfigured", Kind: errs.KindRuntime, Operation: "flow.pull", Summary: "Flow pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow pull before retrying."}
	}
	if input.Workspace == "" {
		return PullOutput{}, &errs.Error{ID: "flow.pull.usage", Kind: errs.KindUsage, Operation: "flow.pull", Summary: "flow pull requires a workspace", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact workspace before pulling.", Validation: []errs.ValidationDetail{{Field: "workspace", Code: "required", Message: "flow pull requires a workspace"}}}
	}
	flow, err := reader.ResolveFlow(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return PullOutput{}, &errs.Error{ID: "flow.pull.resolve", Kind: errs.KindOperation, Operation: "flow.pull", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if input.Preview {
		previewer, ok := writer.(interface {
			PreviewFlow(context.Context, PullInput, Record) (value.AcquisitionPlan, error)
		})
		if !ok {
			return PullOutput{}, &errs.Error{ID: "flow.pull.preview", Kind: errs.KindRuntime, Operation: "flow.pull", Summary: "Acquisition preview is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure read-only artifact preflight."}
		}
		plan, err := previewer.PreviewFlow(ctx, input, flow)
		if err != nil {
			return PullOutput{}, err
		}
		return PullOutput{Status: "preview", Workspace: input.WorkspaceName, Flow: flow, Preview: &plan}, nil
	}
	download, err := reader.DownloadFlow(ctx, flow.LUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow and target site, then pull again.")
		return PullOutput{}, &errs.Error{ID: "flow.pull.download", Kind: errs.KindOperation, Operation: "flow.pull", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow download failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	lineage, lineageErr := reader.CaptureLineage(ctx, PullLineageRequest{Kind: "flow", RESTLUID: flow.LUID, Direction: "both", Depth: 1})
	warnings := []string{}
	if lineageErr != nil {
		lineage.Complete = false
		if lineage.Failure == nil {
			failure := value.LineageFailure{Provider: "tableau-metadata", RootKind: "flow", RootRESTLUID: flow.LUID, RequestID: errs.TableauRequestID(lineageErr)}
			lineage.Failure = &failure
		}
		if lineage.Direction == "" {
			lineage.Direction = "both"
		}
		if lineage.Depth == 0 {
			lineage.Depth = 1
		}
		if len(lineage.Warnings) == 0 {
			lineage.Warnings = []string{"Lineage capture was incomplete. Use lineage.pull or --full for bounded diagnostics; confirmed graph evidence was retained, but counts are unavailable."}
		}
	}
	warnings = append(warnings, lineage.Warnings...)
	result, err := writer.WriteFlow(ctx, PullArtifact{Workspace: input.Workspace, Filename: download.Filename, Content: download.Content, Name: flow.Name, TableauID: flow.LUID, Environment: input.Environment, Site: input.Site, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, ProjectName: flow.ProjectPath, ProjectID: flow.ProjectLUID, FileType: flow.FileType, Lineage: lineage, Overwrite: input.Overwrite})
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the workspace and artifact target, then pull again.")
		return PullOutput{}, &errs.Error{ID: "flow.pull.write", Kind: errs.KindOperation, Operation: "flow.pull", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow artifact write failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if err := pullNormalizeArtifactPaths(&result); err != nil {
		return PullOutput{}, err
	}
	result.LineageStatus = pullLineageStatus(lineage)
	result.NodeCount = len(lineage.Nodes)
	result.EdgeCount = len(lineage.Edges)
	result.CountsKnown = lineageErr == nil && lineage.Complete
	warnings = append(warnings, result.Warnings...)
	return PullOutput{Source: &value.SourceContext{Environment: input.Environment, Site: input.Site}, Workspace: input.WorkspaceName, Status: "pulled", Flow: flow, Artifact: result, Warnings: warnings, compactWarnings: append([]string{}, result.Warnings...), RequestID: download.TableauRequestID, Help: []string{commandhint.SourceUpdate(input.Environment, input.WorkspaceName, "flow", flow.LUID, flow.ProjectLUID)}}, nil
}
func pullLineageStatus(value PullLineage) string {
	if value.Complete {
		return "complete"
	}
	return "incomplete"
}

func pullNormalizeArtifactPaths(result *PullArtifactResult) error {
	paths := []struct {
		name  string
		value *string
	}{
		{name: "artifact path", value: &result.Path},
		{name: "canonical path", value: &result.CanonicalPath},
		{name: "lineage path", value: &result.LineagePath},
	}
	for _, candidate := range paths {
		name, value := candidate.name, candidate.value
		if *value == "" {
			continue
		}
		if pathspec.IsAbs(*value) {
			return &errs.Error{ID: "flow.pull.normalize", Kind: errs.KindOperation, Operation: "flow.pull", Summary: "Flow artifact path normalization failed.", Cause: errors.New("flow artifact writer returned an absolute " + name), Retryable: errs.Bool(false), CorrectiveAction: "Report this flow artifact writer defect; artifact paths must be workspace-relative."}
		}
		normalized := strings.ReplaceAll(*value, "\\", "/")
		for segment := range strings.SplitSeq(normalized, "/") {
			if segment == ".." {
				return &errs.Error{ID: "flow.pull.normalize", Kind: errs.KindOperation, Operation: "flow.pull", Summary: "Flow artifact path normalization failed.", Cause: errors.New("flow artifact writer returned an escaping " + name), Retryable: errs.Bool(false), CorrectiveAction: "Report this flow artifact writer defect; artifact paths must stay within the workspace."}
			}
		}
		*value = normalized
	}
	return nil
}

// ValidatePullInput checks caller-controlled arguments before dependency setup.
func ValidatePullInput(input PullInput) error {
	selector := input.Selector
	if strings.TrimSpace(string(selector.LUID)) == "" && (strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "") {
		return errs.New(errs.KindUsage, "flow selection requires a LUID or exact name and project path")
	}
	if selector.LUID != "" && (selector.Name != "" || selector.ProjectPath != "") {
		return errs.New(errs.KindUsage, "a LUID cannot be combined with name or project selectors")
	}
	return nil
}
