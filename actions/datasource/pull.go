package datasource

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pathspec"
	"github.com/ahillspace/tadx/internal/value"
	"path"
	"strings"
)

// PullReader owns exact datasource resolution, native download, and bounded lineage.
type PullReader interface {
	Resolver
	DownloadDatasource(context.Context, string) (Download, error)
	CaptureLineage(context.Context, LineageRequest) (Lineage, error)
}

// ArtifactWriter materializes one recoverable datasource artifact.
type ArtifactWriter interface {
	WriteDatasource(context.Context, PullArtifact) (PullArtifactResult, error)
}

func Pull(ctx context.Context, reader PullReader, writer ArtifactWriter, input PullInput) (PullOutput, error) {
	if err := ValidatePullInput(input); err != nil {
		return PullOutput{}, err
	}
	if reader == nil || writer == nil {
		return PullOutput{}, &errs.Error{ID: "datasource.pull.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.pull", Summary: "Datasource pull is not configured.", Retryable: new(false), CorrectiveAction: "Configure datasource pull before retrying."}
	}
	if strings.TrimSpace(input.Workspace) == "" {
		return PullOutput{}, pullUsage("workspace", "datasource pull requires a workspace")
	}
	item, err := reader.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		return PullOutput{}, pullOperationError("datasource.pull.resolve", "Datasource resolution failed.", "Review the exact datasource selector, then retry.", input, "", err)
	}
	if input.Preview {
		previewer, ok := writer.(interface {
			PreviewDatasource(context.Context, PullInput, Record) (value.AcquisitionPlan, error)
		})
		if !ok {
			return PullOutput{}, &errs.Error{ID: "datasource.pull.preview", Kind: errs.KindRuntime, Operation: "datasource.pull", Summary: "Acquisition preview is not configured.", Retryable: new(false), CorrectiveAction: "Configure read-only artifact preflight."}
		}
		plan, err := previewer.PreviewDatasource(ctx, input, item)
		if err != nil {
			return PullOutput{}, err
		}
		return PullOutput{Status: "preview", Workspace: input.WorkspaceName, Datasource: item, Preview: &plan}, nil
	}
	download, err := reader.DownloadDatasource(ctx, item.LUID)
	if err != nil {
		return PullOutput{}, pullOperationError("datasource.pull.download", "Datasource download failed.", "Review the exact datasource and target site, then pull again.", input, item.LUID, err)
	}
	lineage, lineageErr := reader.CaptureLineage(ctx, LineageRequest{Kind: "published_datasource", RESTLUID: item.LUID, Direction: "both", Depth: 1})
	warnings := []string{}
	if lineageErr != nil {
		lineage.Complete = false
		if lineage.Failure == nil {
			failure := value.LineageFailure{Provider: "tableau-metadata", RootKind: "published_datasource", RootRESTLUID: item.LUID, RequestID: errs.TableauRequestID(lineageErr)}
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
	result, err := writer.WriteDatasource(ctx, PullArtifact{
		Workspace: input.Workspace, Filename: download.Filename, Content: download.Content,
		Name: item.Name, TableauID: item.LUID, Environment: input.Environment, Site: input.Site,
		ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, ProjectName: item.ProjectPath,
		ProjectID: item.ProjectLUID, Lineage: lineage, Overwrite: input.Overwrite,
	})
	if err != nil {
		return PullOutput{}, pullOperationError("datasource.pull.write", "Datasource artifact write failed.", "Review the workspace and artifact target, then pull again.", input, item.LUID, err)
	}
	if err := pullNormalizeArtifactPaths(&result); err != nil {
		return PullOutput{}, err
	}
	// Single-source lineage completeness so LineageStatus and CountsKnown can never disagree.
	// lineage.Complete is already forced false on any capture error above, so a partial
	// capture (no error, Complete == false) reports incomplete with counts unknown.
	result.LineageStatus = "incomplete"
	if lineage.Complete {
		result.LineageStatus = "complete"
	}
	result.NodeCount, result.EdgeCount = len(lineage.Nodes), len(lineage.Edges)
	result.CountsKnown = lineageErr == nil && lineage.Complete
	warnings = append(warnings, result.Warnings...)
	return PullOutput{Source: &value.SourceContext{Environment: input.Environment, Site: input.Site}, Workspace: input.WorkspaceName, Status: "pulled", Datasource: item, Artifact: result, Warnings: warnings, compactWarnings: append([]string{}, result.Warnings...), RequestID: download.TableauRequestID, Help: []string{commandhint.SourceUpdate(input.Environment, input.WorkspaceName, "datasource", item.LUID, item.ProjectLUID)}}, nil
}

func pullNormalizeArtifactPaths(result *PullArtifactResult) error {
	for _, candidate := range []struct {
		name  string
		value *string
	}{{"artifact path", &result.Path}, {"canonical path", &result.CanonicalPath}, {"lineage path", &result.LineagePath}} {
		if *candidate.value == "" {
			continue
		}
		if pathspec.IsAbs(*candidate.value) {
			return &errs.Error{ID: "datasource.pull.normalize", Kind: errs.KindOperation, Operation: "datasource.pull", Summary: "Datasource artifact path normalization failed.", Cause: errors.New("datasource artifact writer returned an absolute " + candidate.name), Retryable: new(false), CorrectiveAction: "Report this datasource artifact writer defect; artifact paths must be workspace-relative."}
		}
		// Clean with the slash-based path package first so a valid path like a/b/../c
		// collapses to a/c, matching the app-layer filepath.Clean+Rel containment check,
		// while a genuine escape (../x or a/../../x) still surfaces a leading ".." segment.
		normalized := path.Clean(strings.ReplaceAll(*candidate.value, "\\", "/"))
		for _, segment := range strings.Split(normalized, "/") {
			if segment == ".." {
				return &errs.Error{ID: "datasource.pull.normalize", Kind: errs.KindOperation, Operation: "datasource.pull", Summary: "Datasource artifact path normalization failed.", Cause: errors.New("datasource artifact writer returned an escaping " + candidate.name), Retryable: new(false), CorrectiveAction: "Report this datasource artifact writer defect; artifact paths must stay within the workspace."}
			}
		}
		*candidate.value = normalized
	}
	return nil
}

func pullUsage(field, message string) error {
	return &errs.Error{ID: "datasource.pull.usage", Kind: errs.KindUsage, Operation: "datasource.pull", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the datasource pull input and retry.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func pullOperationError(id, summary, corrective string, input PullInput, resource string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, corrective)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.pull", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidatePullInput checks caller-controlled arguments before dependency setup.
func ValidatePullInput(input PullInput) error {
	selector := input.Selector
	if strings.TrimSpace(string(selector.LUID)) == "" && (strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "") {
		return errs.New(errs.KindUsage, "datasource selection requires a LUID or exact name and project path")
	}
	if selector.LUID != "" && (selector.Name != "" || selector.ProjectPath != "") {
		return errs.New(errs.KindUsage, "a LUID cannot be combined with name or project selectors")
	}
	return nil
}
