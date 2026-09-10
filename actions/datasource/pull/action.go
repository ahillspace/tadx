package pull

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"path"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/pathspec"
)

// Reader owns exact datasource resolution, native download, and bounded lineage.
type Reader interface {
	ResolveDatasource(context.Context, identity.Selector) (Datasource, error)
	DownloadDatasource(context.Context, string) (Download, error)
	CaptureLineage(context.Context, LineageRequest) (Lineage, error)
}

// Writer materializes one recoverable datasource artifact.
type Writer interface {
	WriteDatasource(context.Context, Artifact) (ArtifactResult, error)
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
		return Output{}, &errs.Error{ID: "datasource.pull.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.pull", Summary: "Datasource pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure datasource pull before retrying."}
	}
	if strings.TrimSpace(input.Workspace) == "" {
		return Output{}, usage("workspace", "datasource pull requires a workspace")
	}
	item, err := a.reader.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		return Output{}, operationError("datasource.pull.resolve", "Datasource resolution failed.", "Review the exact datasource selector, then retry.", input, "", err)
	}
	download, err := a.reader.DownloadDatasource(ctx, item.LUID)
	if err != nil {
		return Output{}, operationError("datasource.pull.download", "Datasource download failed.", "Review the exact datasource and target site, then pull again.", input, item.LUID, err)
	}
	lineage, lineageErr := a.reader.CaptureLineage(ctx, LineageRequest{Kind: "published_datasource", RESTLUID: item.LUID, Direction: "both", Depth: 1})
	warnings := []string{}
	if lineageErr != nil {
		lineage = Lineage{Complete: false, Direction: "both", Depth: 1}
		warnings = append(warnings, "Lineage capture was incomplete. Use lineage.pull or --full for bounded diagnostics.")
	}
	result, err := a.writer.WriteDatasource(ctx, Artifact{
		Workspace: input.Workspace, Filename: download.Filename, Content: download.Content,
		Name: item.Name, TableauID: item.LUID, Environment: input.Environment, Site: input.Site,
		ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, ProjectName: item.ProjectPath,
		ProjectID: item.ProjectLUID, Lineage: lineage, Overwrite: input.Overwrite,
	})
	if err != nil {
		return Output{}, operationError("datasource.pull.write", "Datasource artifact write failed.", "Review the workspace and artifact target, then pull again.", input, item.LUID, err)
	}
	if err := normalizeArtifactPaths(&result); err != nil {
		return Output{}, err
	}
	// Single-source lineage completeness so LineageStatus and CountsKnown can never disagree.
	// lineage.Complete is already forced false on any capture error above, so a partial
	// capture (no error, Complete == false) reports incomplete with counts unknown.
	result.LineageStatus = "incomplete"
	if lineage.Complete {
		result.LineageStatus = "complete"
	}
	result.NodeCount, result.EdgeCount = len(lineage.Nodes), len(lineage.Edges)
	result.CountsKnown = lineage.Complete
	warnings = append(warnings, result.Warnings...)
	return Output{Workspace: input.WorkspaceName, Status: "pulled", Datasource: item, Artifact: result, Warnings: warnings, RequestID: download.TableauRequestID, Help: []string{commandhint.SourceUpdate(input.Environment, input.WorkspaceName, "datasource", item.LUID, item.ProjectLUID)}}, nil
}

func normalizeArtifactPaths(result *ArtifactResult) error {
	for _, candidate := range []struct {
		name  string
		value *string
	}{{"artifact path", &result.Path}, {"canonical path", &result.CanonicalPath}, {"lineage path", &result.LineagePath}} {
		if *candidate.value == "" {
			continue
		}
		if pathspec.IsAbs(*candidate.value) {
			return &errs.Error{ID: "datasource.pull.normalize", Kind: errs.KindOperation, Operation: "datasource.pull", Summary: "Datasource artifact path normalization failed.", Cause: errors.New("datasource artifact writer returned an absolute " + candidate.name), Retryable: errs.Bool(false), CorrectiveAction: "Report this datasource artifact writer defect; artifact paths must be workspace-relative."}
		}
		// Clean with the slash-based path package first so a valid path like a/b/../c
		// collapses to a/c, matching the app-layer filepath.Clean+Rel containment check,
		// while a genuine escape (../x or a/../../x) still surfaces a leading ".." segment.
		normalized := path.Clean(strings.ReplaceAll(*candidate.value, "\\", "/"))
		for _, segment := range strings.Split(normalized, "/") {
			if segment == ".." {
				return &errs.Error{ID: "datasource.pull.normalize", Kind: errs.KindOperation, Operation: "datasource.pull", Summary: "Datasource artifact path normalization failed.", Cause: errors.New("datasource artifact writer returned an escaping " + candidate.name), Retryable: errs.Bool(false), CorrectiveAction: "Report this datasource artifact writer defect; artifact paths must stay within the workspace."}
			}
		}
		*candidate.value = normalized
	}
	return nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "datasource.pull.usage", Kind: errs.KindUsage, Operation: "datasource.pull", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the datasource pull input and retry.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func operationError(id, summary, corrective string, input Input, resource string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, corrective)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.pull", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
