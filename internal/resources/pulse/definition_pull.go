package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	definition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
	"github.com/ahillspace/tadx/internal/value"
)

// DefinitionPullPort translates bounded native inventory and managed artifact writes.
type DefinitionPullPort struct {
	Client  *tableaupulse.Client
	Manager *artifact.PulseDefinitionManager
}

func (p DefinitionPullPort) GetDefinition(ctx context.Context, luid string) (definition.PullDefinition, error) {
	item, err := p.Client.GetDefinition(ctx, luid)
	if err != nil {
		return definition.PullDefinition{}, err
	}
	result := definition.PullDefinition{LUID: item.LUID, Name: item.Name, DatasourceLUID: item.DatasourceLUID, Configuration: append([]byte(nil), item.Configuration...), RequestID: item.TableauRequestID}
	totalBytes := len(item.Configuration)
	seenIDs, seenTokens := map[string]bool{}, map[string]bool{"": true}
	token := ""
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := p.Client.ListMetrics(ctx, luid, tableaupulse.PageRequest{PageSize: 100, PageToken: token})
		if err != nil {
			return result, err
		}
		if len(page.Metrics) > 100 {
			return result, errors.New("Pulse metric page exceeded its requested bound")
		}
		for _, summary := range page.Metrics {
			if summary.LUID == "" || seenIDs[summary.LUID] || summary.DefinitionLUID != luid {
				return result, errors.New("Pulse metric inventory contains a duplicate or mismatched identity")
			}
			seenIDs[summary.LUID] = true
			metric, err := p.Client.GetMetric(ctx, summary.LUID)
			if err != nil {
				return result, err
			}
			if metric.DefinitionLUID != luid {
				return result, errors.New("Pulse metric changed definition while pulling")
			}
			specification, err := json.Marshal(metric.Specification)
			if err != nil {
				return result, err
			}
			totalBytes += len(specification) + len(metric.LUID) + len(luid) + 128
			if totalBytes > artifact.MaxPulseBundleBytes {
				return result, errors.New("Pulse bundle exceeds its 32 MiB bound; no artifact was written")
			}
			result.Metrics = append(result.Metrics, definition.PullMetric{LUID: metric.LUID, DefinitionLUID: luid, IsDefault: summary.IsDefault || metric.IsDefault, Specification: specification})
		}
		if page.NextPageToken == "" {
			result.MetricsComplete = true
			return result, nil
		}
		if seenTokens[page.NextPageToken] || strings.TrimSpace(page.NextPageToken) == "" {
			return result, errors.New("Pulse metric inventory repeated its continuation token")
		}
		seenTokens[page.NextPageToken] = true
		token = page.NextPageToken
	}
	return result, errors.New("Pulse metric inventory exceeded 100 pages; no bundle was written")
}

func (p DefinitionPullPort) WriteDefinition(ctx context.Context, input definition.PullArtifact) (definition.PullArtifactResult, error) {
	bundle := definitionPullBundle(input)
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return definition.PullArtifactResult{}, err
	}
	result, err := p.Manager.Pull(ctx, artifact.PulseDefinitionPull{Workspace: input.Workspace, Configuration: input.Configuration, Bundle: append(data, '\n'), Overwrite: input.Overwrite, Metadata: artifact.PulseDefinitionMetadata{Kind: "pulse-definition", Name: input.Name, TableauID: input.DefinitionLUID, DatasourceLUID: input.DatasourceLUID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site}})
	if err != nil {
		return definition.PullArtifactResult{}, err
	}
	return definition.PullArtifactResult{Path: result.WorkspaceRelativePath, CanonicalPath: result.CanonicalPath, BaselineFingerprint: result.BaselineFingerprint}, nil
}

func definitionPullBundle(input definition.PullArtifact) artifact.PulseBundle {
	bundle := artifact.PulseBundle{Version: 1, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, DefinitionLUID: input.DefinitionLUID, DatasourceReferences: []string{input.DatasourceLUID}, Definition: input.Configuration, Metrics: make([]artifact.PulseBundleMetric, len(input.Metrics))}
	for i, metric := range input.Metrics {
		bundle.Metrics[i] = artifact.PulseBundleMetric{LUID: metric.LUID, DefinitionLUID: metric.DefinitionLUID, IsDefault: metric.IsDefault, Specification: metric.Specification}
	}
	return bundle
}

func (p DefinitionPullPort) PreviewDefinition(ctx context.Context, input definition.PullInput, item definition.PullDefinition) (value.AcquisitionPlan, error) {
	if input.Environment == "" || input.Site == "" {
		return value.AcquisitionPlan{}, &errs.Error{ID: "pulse.definition.pull.preview", Kind: errs.KindOperation, Operation: "pulse.definition.pull", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition artifact requires source environment and site identity.", Retryable: errs.Bool(false), CorrectiveAction: "Configure a complete Pulse source target before pulling.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	bundle := definitionPullBundle(definition.PullArtifact{ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, DefinitionLUID: item.LUID, DatasourceLUID: item.DatasourceLUID, Configuration: item.Configuration, Metrics: item.Metrics})
	encoded, err := json.Marshal(bundle)
	if err == nil {
		_, err = artifact.DecodePulseBundle(encoded)
	}
	if err != nil {
		return value.AcquisitionPlan{}, &errs.Error{ID: "pulse.definition.pull.preview", Kind: errs.KindOperation, Operation: "pulse.definition.pull", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition bundle validation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve incomplete definition or metric specifications before pulling.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	target, err := artifact.PreviewPull(ctx, artifact.PullPreview{Workspace: input.Workspace, Kind: "pulse-definition", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, &errs.Error{ID: "pulse.definition.pull.preview", Kind: errs.KindOperation, Operation: "pulse.definition.pull", Resource: item.LUID, Summary: "Local acquisition preflight failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the local artifact conflict or workspace prerequisite, then retry."}
	}
	count := len(item.Metrics)
	return value.AcquisitionPlan{Status: "preview", Operation: "pulse.definition.pull", Workspace: input.WorkspaceName, Environment: input.Environment, Site: input.Site, Target: value.AcquisitionTarget{Kind: "pulse-definition", LUID: item.LUID, Name: item.Name, Path: target.Path, Exists: target.Exists, Overwrite: input.Overwrite}, MetricCount: &count, Limitations: []string{"Execution retrieves the definition and saved metric specifications again and rechecks local conflicts. Filesystem write permission is not verified by this preview."}}, nil
}
