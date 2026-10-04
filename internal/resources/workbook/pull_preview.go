package workbook

import (
	"context"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

func previewWorkbookTarget(ctx context.Context, input artifact.PullPreview) (value.AcquisitionTarget, error) {
	result, err := artifact.PreviewPull(ctx, input)
	if err != nil {
		return value.AcquisitionTarget{}, &errs.Error{ID: "workbook.pull.preview", Kind: errs.KindOperation, Operation: "workbook.pull", Resource: input.LUID, Summary: "Local acquisition preflight failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the local artifact conflict or workspace prerequisite, then retry."}
	}
	return value.AcquisitionTarget{Kind: input.Kind, ResourceKind: input.ResourceKind, LUID: input.LUID, Name: input.Name, Path: result.Path, Exists: result.Exists, Overwrite: input.Overwrite}, nil
}

func (w PullWriterPort) PreviewWorkbook(ctx context.Context, input workbook.PullInput, item workbook.Record, references []workbook.PublishedDatasource) (value.AcquisitionPlan, error) {
	target, err := previewWorkbookTarget(ctx, artifact.PullPreview{Workspace: input.Workspace, Kind: "workbook", Name: item.Name, LUID: item.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Overwrite: input.Overwrite})
	if err != nil {
		return value.AcquisitionPlan{}, err
	}
	plan := value.AcquisitionPlan{Status: "preview", Operation: "workbook.pull", Workspace: input.WorkspaceName, Environment: input.Environment, Site: input.Site, Target: target, Limitations: []string{"Execution rechecks local conflicts. Native payload validity, download permission, optional lineage availability, and filesystem write permission are not verified by this preview."}}
	plan.IncludeExtract, plan.IncludePDS = input.IncludeExtract, input.IncludePDS
	plan.Direction, plan.Depth = "both", 1
	for _, reference := range references {
		dependency, err := previewWorkbookTarget(ctx, artifact.PullPreview{Workspace: input.Workspace, Kind: "datasource", Name: reference.Name, LUID: reference.LUID, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID})
		if err != nil {
			return value.AcquisitionPlan{}, err
		}
		plan.Dependencies = append(plan.Dependencies, dependency)
	}
	return plan, nil
}
