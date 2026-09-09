package pull

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/pathspec"
)

type Reader interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
	DownloadFlow(context.Context, string) (Download, error)
	CaptureLineage(context.Context, LineageRequest) (Lineage, error)
}
type Writer interface {
	WriteFlow(context.Context, Artifact) (ArtifactResult, error)
}
type Action struct {
	reader Reader
	writer Writer
}

func New(reader Reader, writer Writer) *Action { return &Action{reader: reader, writer: writer} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, &errs.Error{ID: "flow.pull.unconfigured", Kind: errs.KindRuntime, Operation: "flow.pull", Summary: "Flow pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow pull before retrying."}
	}
	if input.Workspace == "" {
		return Output{}, &errs.Error{ID: "flow.pull.usage", Kind: errs.KindUsage, Operation: "flow.pull", Summary: "flow pull requires a workspace", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact workspace before pulling.", Validation: []errs.ValidationDetail{{Field: "workspace", Code: "required", Message: "flow pull requires a workspace"}}}
	}
	flow, err := a.reader.ResolveFlow(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.pull.resolve", Kind: errs.KindOperation, Operation: "flow.pull", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	download, err := a.reader.DownloadFlow(ctx, flow.LUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow and target site, then pull again.")
		return Output{}, &errs.Error{ID: "flow.pull.download", Kind: errs.KindOperation, Operation: "flow.pull", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow download failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	lineage, lineageErr := a.reader.CaptureLineage(ctx, LineageRequest{Kind: "flow", RESTLUID: flow.LUID, Direction: "both", Depth: 1})
	warnings := []string{}
	if lineageErr != nil {
		lineage = Lineage{Complete: false, Direction: "both", Depth: 1}
		warnings = append(warnings, "Lineage capture was incomplete. Use lineage.pull or --full for bounded diagnostics.")
	}
	result, err := a.writer.WriteFlow(ctx, Artifact{Workspace: input.Workspace, Filename: download.Filename, Content: download.Content, Name: flow.Name, TableauID: flow.LUID, Environment: input.Environment, Site: input.Site, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, ProjectName: flow.ProjectPath, ProjectID: flow.ProjectLUID, FileType: flow.FileType, Lineage: lineage, Overwrite: input.Overwrite})
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the workspace and artifact target, then pull again.")
		return Output{}, &errs.Error{ID: "flow.pull.write", Kind: errs.KindOperation, Operation: "flow.pull", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow artifact write failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if err := normalizeArtifactPaths(&result); err != nil {
		return Output{}, err
	}
	result.LineageStatus = lineageStatus(lineage)
	result.NodeCount = len(lineage.Nodes)
	result.EdgeCount = len(lineage.Edges)
	result.CountsKnown = lineageErr == nil
	warnings = append(warnings, result.Warnings...)
	return Output{Status: "pulled", Flow: flow, Artifact: result, Warnings: warnings, RequestID: download.TableauRequestID, Help: []string{commandhint.Target(input.Environment, input.WorkspaceName, "content", "flow", "publish", "--artifact", result.Path, "--project-id", flow.ProjectLUID, "--overwrite", "--preview")}}, nil
}
func lineageStatus(value Lineage) string {
	if value.Complete {
		return "complete"
	}
	return "incomplete"
}

func normalizeArtifactPaths(result *ArtifactResult) error {
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
		for _, segment := range strings.Split(normalized, "/") {
			if segment == ".." {
				return &errs.Error{ID: "flow.pull.normalize", Kind: errs.KindOperation, Operation: "flow.pull", Summary: "Flow artifact path normalization failed.", Cause: errors.New("flow artifact writer returned an escaping " + name), Retryable: errs.Bool(false), CorrectiveAction: "Report this flow artifact writer defect; artifact paths must stay within the workspace."}
			}
		}
		*value = normalized
	}
	return nil
}
