package workspace

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/actions/batchinput"
	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
)

type BatchArtifactResolver interface {
	ResolveBatchArtifact(context.Context, workspaceaction.DeleteArtifactInput) (workspaceaction.Workspace, workspaceaction.ArtifactTarget, error)
}

func prepareArtifactTarget(ctx context.Context, deps Dependencies, input workspaceaction.DeleteArtifactInput) (workspaceaction.Workspace, workspaceaction.ArtifactTarget, error) {
	if err := batchinput.Authorize(ctx, deps.BatchTargets, ""); err != nil {
		return workspaceaction.Workspace{}, workspaceaction.ArtifactTarget{}, err
	}
	resolver, ok := deps.BatchTargets.(BatchArtifactResolver)
	if !ok {
		return workspaceaction.Workspace{}, workspaceaction.ArtifactTarget{}, errors.New("authoritative local artifact batch preparation is not configured")
	}
	workspace, artifact, err := resolver.ResolveBatchArtifact(ctx, input)
	if err != nil {
		return workspace, artifact, err
	}
	if workspace.Name == "" || workspace.ID == "" || workspace.Root == "" || artifact.Path == "" || artifact.Kind == "" || artifact.LUID == "" {
		return workspace, artifact, errors.New("local artifact batch preparation returned an incomplete target")
	}
	batchinput.Record(ctx, batchinput.Target{WorkspaceID: workspace.ID, WorkspaceRoot: workspace.Root, ResourceKind: artifact.Kind, ResourceID: artifact.Path})
	return workspace, artifact, nil
}

func prepareArtifactMove(ctx context.Context, deps Dependencies, input *workspaceaction.MoveInput) error {
	if !batchinput.Active(ctx) {
		return nil
	}
	workspace, artifact, err := prepareArtifactTarget(ctx, deps, workspaceaction.DeleteArtifactInput{Workspace: input.SourceWorkspace, Kind: input.Kind, LUID: input.LUID, Path: input.Path})
	if err != nil {
		return err
	}
	input.SourceWorkspace, input.Path = workspace.Name, artifact.Path
	input.Kind, input.LUID = "", ""
	input.ExpectedWorkspaceID, input.ExpectedArtifact = workspace.ID, &artifact
	return nil
}

func prepareArtifactDelete(ctx context.Context, deps Dependencies, input *workspaceaction.DeleteArtifactInput) error {
	if !batchinput.Active(ctx) {
		return nil
	}
	workspace, artifact, err := prepareArtifactTarget(ctx, deps, *input)
	if err != nil {
		return err
	}
	input.Workspace, input.Path = workspace.Name, artifact.Path
	input.Kind, input.LUID = "", ""
	input.ExpectedWorkspaceID, input.ExpectedArtifact = workspace.ID, &artifact
	return nil
}
