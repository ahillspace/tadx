package app

import (
	"context"
	"encoding/json"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	definitionpull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

func acquisitionTarget(ctx context.Context, operation string, input artifact.PullPreview) (value.AcquisitionTarget, error) {
	result, err := artifact.PreviewPull(ctx, input)
	if err != nil {
		return value.AcquisitionTarget{}, &errs.Error{ID: operation + ".preview", Kind: errs.KindOperation, Operation: operation, Resource: input.LUID, Summary: "Local acquisition preflight failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the local artifact conflict or workspace prerequisite, then retry."}
	}
	return value.AcquisitionTarget{Kind: input.Kind, ResourceKind: input.ResourceKind, LUID: input.LUID, Name: input.Name, Path: result.Path, Exists: result.Exists, Overwrite: input.Overwrite}, nil
}

func acquisitionPlan(operation, workspace, environment, site string, target value.AcquisitionTarget) value.AcquisitionPlan {
	return value.AcquisitionPlan{Status: "preview", Operation: operation, Workspace: workspace, Environment: environment, Site: site, Target: target, Limitations: []string{"Execution rechecks local conflicts. Native payload validity, download permission, optional lineage availability, and filesystem write permission are not verified by this preview."}}
}

func (w artifactWriter) PreviewWorkbook(ctx context.Context, input workbookops.PullInput, item workbookops.Record, references []workbookops.PublishedDatasource) (value.AcquisitionPlan, error) {
	target, err := acquisitionTarget(ctx, "workbook.pull", artifact.PullPreview{Workspace: input.Workspace, Kind: "workbook", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, err
	}
	plan := acquisitionPlan("workbook.pull", input.WorkspaceName, input.Environment, input.Site, target)
	plan.IncludeExtract, plan.IncludePDS = input.IncludeExtract, input.IncludePDS
	plan.Direction, plan.Depth = "both", 1
	for _, reference := range references {
		dependency, err := acquisitionTarget(ctx, "workbook.pull", artifact.PullPreview{Workspace: input.Workspace, Kind: "datasource", Name: reference.Name, LUID: reference.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID})
		if err != nil {
			return value.AcquisitionPlan{}, err
		}
		plan.Dependencies = append(plan.Dependencies, dependency)
	}
	return plan, nil
}

func (w datasourceArtifactWriter) PreviewDatasource(ctx context.Context, input datasourceops.PullInput, item datasourceops.Record) (value.AcquisitionPlan, error) {
	target, err := acquisitionTarget(ctx, "datasource.pull", artifact.PullPreview{Workspace: input.Workspace, Kind: "datasource", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, err
	}
	plan := acquisitionPlan("datasource.pull", input.WorkspaceName, input.Environment, input.Site, target)
	plan.Direction, plan.Depth = "both", 1
	return plan, nil
}

func (w flowArtifactWriter) PreviewFlow(ctx context.Context, input flowops.PullInput, item flowops.Record) (value.AcquisitionPlan, error) {
	if item.ProjectLUID == "" || item.ProjectPath == "" {
		return value.AcquisitionPlan{}, capabilitySetupError("flow.pull.preview", "flow.pull", input.Environment, input.Site, "Flow artifact requires complete source project identity.", "Resolve the source project before pulling.", nil)
	}
	target, err := acquisitionTarget(ctx, "flow.pull", artifact.PullPreview{Workspace: input.Workspace, Kind: "flow", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, err
	}
	plan := acquisitionPlan("flow.pull", input.WorkspaceName, input.Environment, input.Site, target)
	plan.Direction, plan.Depth = "both", 1
	return plan, nil
}

func (w lineageArtifactWriter) PreviewLineage(ctx context.Context, input lineagepull.Input, item lineagepull.Resource) (value.AcquisitionPlan, error) {
	target, err := acquisitionTarget(ctx, "lineage.pull", artifact.PullPreview{Workspace: input.Workspace, Kind: "lineage", ResourceKind: item.Kind, Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, err
	}
	plan := acquisitionPlan("lineage.pull", input.WorkspaceName, input.Environment, input.Site, target)
	plan.Direction, plan.Depth = input.Direction, input.Depth
	plan.Limitations = []string{"The graph is not captured during preview. Metadata API availability, completeness, and filesystem write permission are checked during execution."}
	return plan, nil
}

func (w pulseDefinitionArtifactWriter) PreviewDefinition(ctx context.Context, input definitionpull.Input, item definitionpull.Definition) (value.AcquisitionPlan, error) {
	if input.Environment == "" || input.Site == "" {
		return value.AcquisitionPlan{}, capabilitySetupError("pulse.definition.pull.preview", "pulse.definition.pull", input.Environment, input.Site, "Pulse definition artifact requires source environment and site identity.", "Configure a complete Pulse source target before pulling.", nil)
	}
	bundle := artifact.PulseBundle{Version: 1, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, DefinitionLUID: item.LUID, DatasourceReferences: []string{item.DatasourceLUID}, Definition: item.Configuration}
	for _, metric := range item.Metrics {
		bundle.Metrics = append(bundle.Metrics, artifact.PulseBundleMetric{LUID: metric.LUID, DefinitionLUID: metric.DefinitionLUID, IsDefault: metric.IsDefault, Specification: metric.Specification})
	}
	encoded, err := json.Marshal(bundle)
	if err == nil {
		_, err = artifact.DecodePulseBundle(encoded)
	}
	if err != nil {
		return value.AcquisitionPlan{}, capabilitySetupError("pulse.definition.pull.preview", "pulse.definition.pull", input.Environment, input.Site, "Pulse definition bundle validation failed.", "Resolve incomplete definition or metric specifications before pulling.", err)
	}
	target, err := acquisitionTarget(ctx, "pulse.definition.pull", artifact.PullPreview{Workspace: input.Workspace, Kind: "pulse-definition", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, err
	}
	plan := acquisitionPlan("pulse.definition.pull", input.WorkspaceName, input.Environment, input.Site, target)
	count := len(item.Metrics)
	plan.MetricCount = &count
	plan.Limitations = []string{"Execution retrieves the definition and saved metric specifications again and rechecks local conflicts. Filesystem write permission is not verified by this preview."}
	return plan, nil
}
