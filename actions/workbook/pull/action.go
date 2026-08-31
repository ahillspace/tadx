package pull

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Reader owns remote workbook resolution and download.
type Reader interface {
	ResolveWorkbook(context.Context, identity.Selector) (Workbook, error)
	DownloadWorkbook(context.Context, string, *bool) (Download, error)
	PublishedDatasources(context.Context, string) ([]PublishedDatasource, error)
	DownloadPublishedDatasource(context.Context, string) (DatasourceDownload, error)
}

// ArtifactWriter owns canonical local persistence and dirty-state protection.
type ArtifactWriter interface {
	WriteWorkbook(context.Context, Artifact) (ArtifactResult, error)
	WriteBundle(context.Context, Artifact, []DatasourceArtifact) (ArtifactResult, error)
}

// Action orchestrates workbook.pull.
type Action struct {
	reader Reader
	writer ArtifactWriter
}

// New creates workbook.pull.
func New(reader Reader, writer ArtifactWriter) *Action {
	return &Action{reader: reader, writer: writer}
}

// Execute resolves, downloads, and materializes one workbook artifact.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, &errs.Error{ID: "workbook.pull.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.pull", Summary: "Workbook pull is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook pulling before retrying."}
	}
	selector := input.Selector
	if selector.LUID == "" && selector.Name == "" && selector.ProjectPath == "" {
		selector = identity.Selector{LUID: identity.LUID(input.LUID), Name: input.Name, ProjectPath: input.ProjectPath}
	}
	if selector.LUID == "" && selector.Name == "" {
		message := "One of workbook LUID or name is required."
		return Output{}, &errs.Error{ID: "workbook.pull.usage", Kind: errs.KindUsage, Operation: "workbook.pull", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Provide --id or --name, then retry.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "one of --id or --name is required"}}}
	}
	workbook, err := a.reader.ResolveWorkbook(ctx, selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workbook selector and target, then retry.")
		return Output{}, &errs.Error{ID: "workbook.pull.resolve", Kind: errs.KindOperation, Operation: "workbook.pull", Environment: input.Environment, Site: input.Site, Summary: "Workbook resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	download, err := a.reader.DownloadWorkbook(ctx, workbook.LUID, input.IncludeExtract)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review workbook access and the exact target, then retry.")
		requestID := download.TableauRequestID
		if requestID == "" {
			requestID = errs.TableauRequestID(err)
		}
		return Output{}, &errs.Error{ID: "workbook.pull.download", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook download failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: requestID}
	}
	references, detectionErr := a.reader.PublishedDatasources(ctx, workbook.LUID)
	var warnings []string
	portability := "unknown"
	if detectionErr != nil {
		if input.IncludePDS {
			retryable, correctiveAction := errs.CompleteRetryAdvice(detectionErr, "Wait for complete Tableau metadata visibility, then retry.")
			return Output{}, &errs.Error{ID: "workbook.pull.references", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource detection was incomplete.", Cause: detectionErr, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(detectionErr)}
		}
		warnings = append(warnings, fmt.Sprintf("Published datasource detection was incomplete; workbook portability remains unknown. Detail: %v", detectionErr))
		references = nil
	} else {
		references, err = normalizePublishedDatasources(references)
		if err != nil {
			retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Wait for complete Tableau metadata visibility, then retry.")
			return Output{}, &errs.Error{ID: "workbook.pull.references", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource detection returned invalid identities.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
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
			dependency, downloadErr := a.reader.DownloadPublishedDatasource(ctx, reference.LUID)
			if downloadErr != nil {
				retryable, correctiveAction := errs.CompleteRetryAdvice(downloadErr, "Review published datasource access and retry the workbook pull.")
				return Output{}, &errs.Error{ID: "workbook.pull.dependency-download", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: reference.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource dependency download failed.", Cause: downloadErr, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(downloadErr)}
			}
			if dependency.LUID == "" || dependency.LUID != reference.LUID {
				return Output{}, &errs.Error{ID: "workbook.pull.dependency-identity", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: reference.LUID, Environment: input.Environment, Site: input.Site, Summary: "Published datasource dependency returned an invalid authoritative identity.", Cause: fmt.Errorf("datasource download returned LUID %q, expected %q", dependency.LUID, reference.LUID), Retryable: errs.Bool(false), CorrectiveAction: "Refresh Tableau metadata and retry the workbook pull."}
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
	workbookArtifact := Artifact{
		Workspace: input.Workspace, Filename: download.Filename, Content: download.Content,
		Name: workbook.Name, TableauID: workbook.LUID, Environment: input.Environment, Site: input.Site,
		ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID,
		ProjectName: workbook.ProjectPath, ProjectID: workbook.ProjectLUID, Portability: portability,
		PublishedDatasources: provenance, DependenciesAcquired: dependenciesAcquired,
		Overwrite: input.Overwrite, TableauRequestID: download.TableauRequestID,
	}
	var artifact ArtifactResult
	if dependenciesAcquired {
		artifact, err = a.writer.WriteBundle(ctx, workbookArtifact, datasourceArtifacts)
	} else {
		artifact, err = a.writer.WriteWorkbook(ctx, workbookArtifact)
	}
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve local artifact conflicts, then retry the complete workbook pull.")
		return Output{}, &errs.Error{ID: "workbook.pull.write", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook artifact transaction failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: download.TableauRequestID}
	}
	dependencies := artifact.Dependencies
	if dependenciesAcquired {
		if len(dependencies) != len(provenance) {
			return Output{}, invalidBundleResult(workbook, input, fmt.Errorf("bundle returned %d datasource artifacts, expected %d", len(dependencies), len(provenance)))
		}
		dependencyByLUID := make(map[string]DependencyArtifactResult, len(dependencies))
		for index := range dependencies {
			dependency := &dependencies[index]
			dependency.LUID = strings.TrimSpace(dependency.LUID)
			dependency.Path = filepath.ToSlash(strings.TrimSpace(dependency.Path))
			dependency.CanonicalPath = filepath.ToSlash(strings.TrimSpace(dependency.CanonicalPath))
			if dependency.LUID == "" || dependency.Path == "" {
				return Output{}, invalidBundleResult(workbook, input, errors.New("bundle datasource result requires LUID and path"))
			}
			if _, exists := dependencyByLUID[dependency.LUID]; exists {
				return Output{}, invalidBundleResult(workbook, input, fmt.Errorf("bundle returned duplicate datasource LUID %q", dependency.LUID))
			}
			dependencyByLUID[dependency.LUID] = *dependency
			warnings = append(warnings, dependency.Warnings...)
		}
		for index := range provenance {
			dependency, exists := dependencyByLUID[provenance[index].LUID]
			if !exists {
				return Output{}, invalidBundleResult(workbook, input, fmt.Errorf("bundle omitted datasource LUID %q", provenance[index].LUID))
			}
			provenance[index].LocalArtifactPath = dependency.Path
			provenance[index].CanonicalPath = dependency.CanonicalPath
			provenance[index].BaselineFingerprint = dependency.BaselineFingerprint
		}
	}
	artifact.Portability = portability
	artifact.PublishedDatasources = provenance
	artifact.DependenciesAcquired = dependenciesAcquired
	artifact.Dependencies = dependencies
	warnings = append(warnings, artifact.Warnings...)
	return Output{Status: "pulled", Workbook: workbook, Artifact: artifact, Warnings: warnings, RequestID: download.TableauRequestID, Help: []string{"tadx content workbook publish --artifact <path> --environment <alias>"}}, nil
}

func invalidBundleResult(workbook Workbook, input Input, cause error) error {
	return &errs.Error{ID: "workbook.pull.write", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: workbook.LUID, Environment: input.Environment, Site: input.Site, Summary: "Workbook artifact transaction returned an invalid result.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the local workspace and retry the complete workbook pull."}
}

func normalizePublishedDatasources(input []PublishedDatasource) ([]PublishedDatasource, error) {
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
