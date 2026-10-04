package app

import (
	"context"
	"encoding/json"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
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

func (w pulseDefinitionArtifactWriter) PreviewDefinition(ctx context.Context, input pulsedefinition.PullInput, item pulsedefinition.PullDefinition) (value.AcquisitionPlan, error) {
	if input.Environment == "" || input.Site == "" {
		return value.AcquisitionPlan{}, capabilitySetupError("pulse.definition.pull.preview", "pulse.definition.pull", input.Environment, input.Site, "Pulse definition artifact requires source environment and site identity.", "Configure a complete Pulse source target before pulling.", nil)
	}
	bundle := pulseDefinitionBundle(pulsedefinition.PullArtifact{ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, DefinitionLUID: item.LUID, DatasourceLUID: item.DatasourceLUID, Configuration: item.Configuration, Metrics: item.Metrics})
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
