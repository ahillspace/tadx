package workbook

import "github.com/ahillspace/tadx/internal/value"

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/pathspec"
)

// PullReader owns remote workbook resolution and download.
type PullReader interface {
	Resolver
	DownloadWorkbook(context.Context, string, *bool) (Download, error)
	CaptureWorkbookLineage(context.Context, LineageRequest) (LineageCapture, error)
	PublishedDatasources(context.Context, string) ([]PublishedDatasource, error)
	DownloadPublishedDatasource(context.Context, string) (DatasourceDownload, error)
}

// ArtifactWriter owns canonical local persistence and dirty-state protection.
type ArtifactWriter interface {
	WriteWorkbook(context.Context, PullArtifact) (PullArtifactResult, error)
	WriteBundle(context.Context, PullArtifact, []DatasourceArtifact) (PullArtifactResult, error)
}

// Pull resolves, downloads, and materializes one workbook artifact.
func Pull(ctx context.Context, reader PullReader, writer ArtifactWriter, input PullInput) (PullOutput, error) {
	if err := ValidatePullInput(input); err != nil {
		return PullOutput{}, err
	}
	if reader == nil || writer == nil {
		return PullOutput{}, &errs.Error{ID: "workbook.pull.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.pull", Summary: "Workbook pull is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook pulling before retrying."}
	}
	selector := input.Selector
	if selector.LUID == "" && selector.Name == "" && selector.ProjectPath == "" {
		selector = identity.Selector{LUID: identity.LUID(input.LUID), Name: input.Name, ProjectPath: input.ProjectPath}
	}
	workbook, err := reader.ResolveWorkbook(ctx, selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workbook selector and target, then retry.")
		return PullOutput{}, &errs.Error{ID: "workbook.pull.resolve", Kind: errs.KindOperation, Operation: "workbook.pull", Environment: input.Environment, Site: input.Site, Summary: "Workbook resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if input.Preview {
		var references []PublishedDatasource
		if input.IncludePDS {
			references, err = reader.PublishedDatasources(ctx, workbook.LUID)
			if err == nil {
				references, err = pullNormalizePublishedDatasources(references)
			}
			if err != nil {
				return PullOutput{}, &errs.Error{ID: "workbook.pull.references", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource detection was incomplete.", Cause: err, Retryable: new(false), CorrectiveAction: "Wait for complete Tableau metadata visibility, then retry."}
			}
		}
		previewer, ok := writer.(interface {
			PreviewWorkbook(context.Context, PullInput, Record, []PublishedDatasource) (value.AcquisitionPlan, error)
		})
		if !ok {
			return PullOutput{}, &errs.Error{ID: "workbook.pull.preview", Kind: errs.KindRuntime, Operation: "workbook.pull", Summary: "Acquisition preview is not configured.", Retryable: new(false), CorrectiveAction: "Configure read-only artifact preflight."}
		}
		plan, err := previewer.PreviewWorkbook(ctx, input, workbook, references)
		if err != nil {
			return PullOutput{}, err
		}
		return PullOutput{Status: "preview", Workspace: input.WorkspaceName, Workbook: workbook, Preview: &plan}, nil
	}
	download, err := reader.DownloadWorkbook(ctx, workbook.LUID, input.IncludeExtract)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review workbook access and the exact target, then retry.")
		requestID := download.TableauRequestID
		if requestID == "" {
			requestID = errs.TableauRequestID(err)
		}
		return PullOutput{}, &errs.Error{ID: "workbook.pull.download", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook download failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: requestID}
	}
	lineage, lineageCountsKnown, lineageStatus, lineageWarnings := pullCaptureAutomaticLineage(ctx, reader, workbook.LUID)
	references, detectionErr := reader.PublishedDatasources(ctx, workbook.LUID)
	warnings := append([]string(nil), lineageWarnings...)
	compactWarnings := make([]string, 0)
	portability := "unknown"
	if detectionErr != nil {
		if input.IncludePDS {
			retryable, correctiveAction := errs.CompleteRetryAdvice(detectionErr, "Wait for complete Tableau metadata visibility, then retry.")
			return PullOutput{}, &errs.Error{ID: "workbook.pull.references", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource detection was incomplete.", Cause: detectionErr, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(detectionErr)}
		}
		warnings = append(warnings, "Published datasource detection was incomplete; workbook portability remains unknown. Retry after Tableau metadata refreshes.")
		references = nil
	} else {
		references, err = pullNormalizePublishedDatasources(references)
		if err != nil {
			retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Wait for complete Tableau metadata visibility, then retry.")
			return PullOutput{}, &errs.Error{ID: "workbook.pull.references", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource detection returned invalid identities.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
		}
		portability = "portable"
		if len(references) > 0 {
			portability = "source-site-bound"
		}
	}

	provenance := make([]PublishedDatasourceRef, len(references))
	for index, reference := range references {
		provenance[index] = PublishedDatasourceRef{LUID: reference.LUID, Name: reference.Name, SourceSite: input.Site}
	}
	var datasourceArtifacts []DatasourceArtifact
	if input.IncludePDS {
		for index, reference := range references {
			dependency, downloadErr := reader.DownloadPublishedDatasource(ctx, reference.LUID)
			if downloadErr != nil {
				retryable, correctiveAction := errs.CompleteRetryAdvice(downloadErr, "Review published datasource access and retry the workbook pull.")
				return PullOutput{}, &errs.Error{ID: "workbook.pull.dependency-download", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: reference.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource dependency download failed.", Cause: downloadErr, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(downloadErr)}
			}
			if dependency.LUID == "" || dependency.LUID != reference.LUID {
				return PullOutput{}, &errs.Error{ID: "workbook.pull.dependency-identity", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: reference.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource dependency returned an invalid authoritative identity.", Cause: fmt.Errorf("datasource download returned LUID %q, expected %q", dependency.LUID, reference.LUID), Retryable: new(false), CorrectiveAction: "Refresh Tableau metadata and retry the workbook pull."}
			}
			datasourceArtifacts = append(datasourceArtifacts, DatasourceArtifact{
				Workspace: input.Workspace, Filename: dependency.Filename, Content: dependency.Content,
				Name: dependency.Name, TableauID: dependency.LUID, Environment: input.Environment, Site: input.Site,
				ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, ProjectName: dependency.ProjectPath,
				ProjectID: dependency.ProjectLUID, Overwrite: false, TableauRequestID: dependency.TableauRequestID,
			})
			provenance[index].Name = dependency.Name
		}
	}
	dependenciesAcquired := input.IncludePDS && len(references) > 0
	workbookArtifact := PullArtifact{
		Workspace: input.Workspace, Filename: download.Filename, Content: download.Content,
		Name: workbook.Name, TableauID: workbook.LUID, Environment: input.Environment, Site: input.Site,
		ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID,
		ProjectName: workbook.ProjectPath, ProjectID: workbook.ProjectLUID, Portability: portability,
		PublishedDatasources: provenance, DependenciesAcquired: dependenciesAcquired,
		Lineage: lineage, LineageCountsKnown: lineageCountsKnown,
		Overwrite: input.Overwrite, TableauRequestID: download.TableauRequestID,
	}
	var artifact PullArtifactResult
	if dependenciesAcquired {
		artifact, err = writer.WriteBundle(ctx, workbookArtifact, datasourceArtifacts)
	} else {
		artifact, err = writer.WriteWorkbook(ctx, workbookArtifact)
	}
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve local artifact conflicts, then retry the complete workbook pull.")
		return PullOutput{}, &errs.Error{ID: "workbook.pull.write", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook artifact transaction failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: download.TableauRequestID}
	}
	dependencies := artifact.Dependencies
	if dependenciesAcquired {
		if len(dependencies) != len(provenance) {
			return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("bundle returned %d datasource artifacts, expected %d", len(dependencies), len(provenance)))
		}
		dependencyByLUID := make(map[string]DependencyArtifactResult, len(dependencies))
		for index := range dependencies {
			dependency := &dependencies[index]
			dependency.LUID = strings.TrimSpace(dependency.LUID)
			dependency.Path, err = pullWorkspaceRelativePath(input.Workspace, dependency.Path)
			if err != nil {
				return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("project datasource artifact path: %w", err))
			}
			dependency.CanonicalPath, err = pullWorkspaceRelativePath(input.Workspace, dependency.CanonicalPath)
			if err != nil {
				return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("project datasource canonical path: %w", err))
			}
			if dependency.LUID == "" || dependency.Path == "" {
				return PullOutput{}, pullInvalidBundleResult(workbook, input, errors.New("bundle datasource result requires LUID and path"))
			}
			if _, exists := dependencyByLUID[dependency.LUID]; exists {
				return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("bundle returned duplicate datasource LUID %q", dependency.LUID))
			}
			dependencyByLUID[dependency.LUID] = *dependency
			warnings = append(warnings, dependency.Warnings...)
			compactWarnings = append(compactWarnings, dependency.Warnings...)
		}
		for index := range provenance {
			dependency, exists := dependencyByLUID[provenance[index].LUID]
			if !exists {
				return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("bundle omitted datasource LUID %q", provenance[index].LUID))
			}
			provenance[index].LocalArtifactPath = dependency.Path
			provenance[index].CanonicalPath = dependency.CanonicalPath
			provenance[index].BaselineFingerprint = dependency.BaselineFingerprint
		}
	}
	artifact.Portability = portability
	artifact.PublishedDatasources = provenance
	artifact.DependenciesAcquired = dependenciesAcquired
	artifact.LineageStatus = lineageStatus
	if lineageCountsKnown {
		nodeCount, edgeCount := len(lineage.Nodes), len(lineage.Edges)
		artifact.LineageNodeCount = &nodeCount
		artifact.LineageEdgeCount = &edgeCount
	}
	artifact.Dependencies = dependencies
	artifact.Path, err = pullWorkspaceRelativePath(input.Workspace, artifact.Path)
	if err != nil {
		return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("project workbook artifact path: %w", err))
	}
	artifact.CanonicalPath, err = pullWorkspaceRelativePath(input.Workspace, artifact.CanonicalPath)
	if err != nil {
		return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("project workbook canonical path: %w", err))
	}
	artifact.LineagePath, err = pullWorkspaceRelativePath(input.Workspace, artifact.LineagePath)
	if err != nil {
		return PullOutput{}, pullInvalidBundleResult(workbook, input, fmt.Errorf("project workbook lineage path: %w", err))
	}
	warnings = append(warnings, artifact.Warnings...)
	compactWarnings = append(compactWarnings, artifact.Warnings...)
	return PullOutput{Source: &value.SourceContext{Environment: input.Environment, Site: input.Site}, Workspace: input.WorkspaceName, Status: "pulled", Workbook: workbook, Artifact: artifact, Warnings: warnings, compactWarnings: compactWarnings, RequestID: download.TableauRequestID, Help: []string{commandhint.SourceUpdate(input.Environment, input.WorkspaceName, "workbook", workbook.LUID, workbook.ProjectLUID)}}, nil
}

func pullCaptureAutomaticLineage(ctx context.Context, reader PullReader, workbookLUID string) (LineageCapture, bool, string, []string) {
	request := LineageRequest{RESTLUID: workbookLUID, Direction: "both", Depth: 1}
	capture, err := reader.CaptureWorkbookLineage(ctx, request)
	if err != nil {
		capture.Direction = request.Direction
		capture.Depth = request.Depth
		capture.Complete = false
		capture.RootMetadataID = strings.TrimSpace(capture.RootMetadataID)
		if capture.Failure == nil {
			failure := value.LineageFailure{Provider: "tableau-metadata", RootKind: "workbook", RootRESTLUID: workbookLUID, RequestID: errs.TableauRequestID(err)}
			capture.Failure = &failure
		}
		if validationErr := pullValidateLineageCapture(capture, workbookLUID); validationErr != nil {
			return pullUnavailableLineage(), false, "unavailable", []string{"Lineage capture was unavailable. The workbook download remains valid; retry after reviewing Metadata API access."}
		}
		if len(capture.Nodes) == 0 && capture.RootMetadataID == "" {
			return pullUnavailableLineage(), false, "unavailable", []string{"Lineage capture was unavailable. The workbook download remains valid; retry after reviewing Metadata API access."}
		}
		warnings := pullBoundedLineageWarnings(capture.Warnings)
		if len(warnings) == 0 {
			warnings = []string{"Lineage capture was incomplete. The workbook download remains valid; confirmed graph evidence was retained, but counts are unavailable."}
		}
		capture.Warnings = warnings
		return capture, false, "incomplete", warnings
	}
	capture.Direction = request.Direction
	capture.Depth = request.Depth
	capture.RootMetadataID = strings.TrimSpace(capture.RootMetadataID)
	if err := pullValidateLineageCapture(capture, workbookLUID); err != nil {
		return pullUnavailableLineage(), false, "unavailable", []string{"Lineage capture returned an invalid or incomplete graph. The workbook download remains valid; retry after Tableau metadata refreshes."}
	}
	warnings := pullBoundedLineageWarnings(capture.Warnings)
	status := "complete"
	if !capture.Complete {
		status = "incomplete"
		if len(warnings) == 0 {
			warnings = []string{"Lineage capture was incomplete. The workbook download remains valid; use lineage.pull after Tableau metadata refreshes."}
		}
	}
	capture.Warnings = warnings
	return capture, true, status, warnings
}

func pullUnavailableLineage() LineageCapture {
	return LineageCapture{Complete: false, Direction: "both", Depth: 1, Nodes: []LineageNode{}, Edges: []LineageEdge{}}
}

func pullValidateLineageCapture(capture LineageCapture, workbookLUID string) error {
	if capture.Direction != "both" || capture.Depth != 1 {
		return errors.New("automatic lineage must use direction both and depth one")
	}
	if len(capture.Nodes) > pullMaxLineageNodes || len(capture.Edges) > pullMaxLineageEdges {
		return errors.New("lineage graph exceeds its artifact bound")
	}
	seen := make(map[string]LineageNode, len(capture.Nodes))
	for index := range capture.Nodes {
		node := &capture.Nodes[index]
		node.MetadataID = strings.TrimSpace(node.MetadataID)
		node.Kind = strings.TrimSpace(node.Kind)
		node.RESTLUID = strings.TrimSpace(node.RESTLUID)
		node.Name = strings.TrimSpace(node.Name)
		if node.MetadataID == "" || node.Kind == "" {
			return errors.New("lineage node omitted identity")
		}
		if _, exists := seen[node.MetadataID]; exists {
			return errors.New("lineage Metadata ID is duplicated")
		}
		seen[node.MetadataID] = *node
	}
	rootID := strings.TrimSpace(capture.RootMetadataID)
	if capture.Complete && rootID == "" {
		return errors.New("complete lineage omitted root Metadata ID")
	}
	if rootID != "" {
		root, exists := seen[rootID]
		if !exists || root.Kind != "workbook" || root.RESTLUID != workbookLUID {
			return errors.New("lineage root does not map to the authoritative workbook")
		}
	}
	for index := range capture.Edges {
		edge := &capture.Edges[index]
		edge.FromMetadataID = strings.TrimSpace(edge.FromMetadataID)
		edge.ToMetadataID = strings.TrimSpace(edge.ToMetadataID)
		edge.Relationship = strings.TrimSpace(edge.Relationship)
		if strings.TrimSpace(edge.Relationship) == "" {
			return errors.New("lineage edge omitted relationship")
		}
		if _, exists := seen[edge.FromMetadataID]; !exists {
			return errors.New("lineage edge omitted source node")
		}
		if _, exists := seen[edge.ToMetadataID]; !exists {
			return errors.New("lineage edge omitted destination node")
		}
	}
	return nil
}

func pullBoundedLineageWarnings(input []string) []string {
	unique := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, warning := range input {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		if len(warning) > pullMaxLineageWarningBytes {
			cut := pullMaxLineageWarningBytes
			for cut > 0 && !utf8.RuneStart(warning[cut]) {
				cut--
			}
			warning = warning[:cut]
		}
		if _, exists := seen[warning]; exists {
			continue
		}
		seen[warning] = struct{}{}
		unique = append(unique, warning)
	}
	sort.Strings(unique)
	if len(unique) > pullMaxOutputWarnings {
		unique = unique[:pullMaxOutputWarnings]
	}
	return unique
}

func pullWorkspaceRelativePath(workspace, path string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if !pathspec.IsAbs(path) {
		cleaned := filepath.ToSlash(filepath.Clean(path))
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return "", errors.New("artifact path escapes the resolved workspace")
		}
		return cleaned, nil
	}
	if workspace == "" {
		return "", errors.New("absolute artifact path requires a resolved workspace")
	}
	relative, err := filepath.Rel(workspace, path)
	if err != nil {
		return "", fmt.Errorf("resolve path relative to workspace: %w", err)
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", errors.New("artifact path escapes the resolved workspace")
	}
	return relative, nil
}

func pullInvalidBundleResult(workbook Record, input PullInput, cause error) error {
	return &errs.Error{ID: "workbook.pull.write", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook artifact transaction returned an invalid result.", Cause: cause, Retryable: new(false), CorrectiveAction: "Inspect the local workspace and retry the complete workbook pull."}
}

func pullNormalizePublishedDatasources(input []PublishedDatasource) ([]PublishedDatasource, error) {
	byLUID := make(map[string]PublishedDatasource, len(input))
	for _, item := range input {
		item.LUID = strings.TrimSpace(item.LUID)
		item.Name = strings.TrimSpace(item.Name)
		if item.LUID == "" {
			return nil, errors.New("published datasource reference requires an authoritative LUID")
		}
		if existing, ok := byLUID[item.LUID]; ok {
			if existing.Name != "" && item.Name != "" && existing.Name != item.Name {
				return nil, fmt.Errorf("published datasource %q returned conflicting names %q and %q", item.LUID, existing.Name, item.Name)
			}
			if existing.Name == "" {
				byLUID[item.LUID] = item
			}
			continue
		}
		byLUID[item.LUID] = item
	}
	result := make([]PublishedDatasource, 0, len(byLUID))
	for _, item := range byLUID {
		result = append(result, item)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].LUID < result[right].LUID })
	return result, nil
}

// ValidatePullInput checks caller-controlled arguments before dependency setup.
func ValidatePullInput(input PullInput) error {
	selector := input.Selector
	if selector.LUID == "" && selector.Name == "" && selector.ProjectPath == "" {
		selector = identity.Selector{LUID: identity.LUID(input.LUID), Name: input.Name, ProjectPath: input.ProjectPath}
	}
	if strings.TrimSpace(string(selector.LUID)) == "" && strings.TrimSpace(selector.Name) == "" {
		return errs.New(errs.KindUsage, "one of workbook LUID or name is required")
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		return errs.New(errs.KindUsage, "a LUID cannot be combined with name or project selectors")
	}
	return nil
}
