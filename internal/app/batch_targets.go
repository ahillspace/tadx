package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/actions/batchinput"
	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

// batchTargetResolver uses the same exact resource adapters as the actions.
// Prepared inputs retain IDs; the actions still revalidate current state.
type batchTargetResolver struct{ runtime *runtimeDependencies }

func resolvePullWorkspace(ctx context.Context, runtime *runtimeDependencies, operation, environment, selector, expectedID, expectedRoot string) (workspacecore.Record, error) {
	workspace, err := (&workspaceRuntime{runtime: runtime}).resolveForEnvironment(ctx, selector, environment)
	if err == nil {
		workspace.Root, err = workspacecore.CanonicalRoot(workspace.Root)
	}
	if err != nil {
		return workspacecore.Record{}, capabilitySetupError(operation+".workspace", operation, environment, "", "Artifact workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	if expectedID != "" && (workspace.ID != expectedID || workspace.Root != expectedRoot) {
		return workspacecore.Record{}, &errs.Error{ID: operation + ".workspace_changed", Kind: errs.KindOperation, Operation: operation, Environment: environment, Summary: "The artifact workspace changed after batch preparation.", Retryable: errs.Bool(false), Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, CorrectiveAction: "Review a new batch plan before pulling."}
	}
	return workspace, nil
}

func (r batchTargetResolver) AuthorizeBatchPreparation(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.runtime.checkManagedCapability(batchinput.Operation(ctx))
}

func (r batchTargetResolver) ResolveBatchArtifact(ctx context.Context, input workspaceaction.DeleteArtifactInput) (workspaceaction.Workspace, workspaceaction.ArtifactTarget, error) {
	if err := batchinput.Authorize(ctx, r, ""); err != nil {
		return workspaceaction.Workspace{}, workspaceaction.ArtifactTarget{}, err
	}
	workspace, err := (&workspaceRuntime{runtime: r.runtime}).resolve(ctx, input.Workspace)
	if err != nil {
		return workspaceaction.Workspace{}, workspaceaction.ArtifactTarget{}, batchPreparationError(ctx, "", "", err)
	}
	workspace.Root, err = workspacecore.CanonicalRoot(workspace.Root)
	if err != nil {
		return workspaceaction.Workspace{}, workspaceaction.ArtifactTarget{}, batchPreparationError(ctx, "", "", err)
	}
	item, err := artifact.Resolve(ctx, workspace.Root, artifact.Selector{Kind: input.Kind, LUID: input.LUID, Path: input.Path, ExpectedWorkspaceID: workspace.ID})
	if err != nil {
		return workspaceaction.Workspace{}, workspaceaction.ArtifactTarget{}, batchPreparationError(ctx, "", "", mapArtifactResolutionError(batchinput.Operation(ctx), workspace.Name, input.LUID, err))
	}
	target := deleteArtifact(item)
	target.WorkspaceID = workspace.ID
	return workspaceIdentity(workspace), target, nil
}

func (r batchTargetResolver) ResolveBatchResource(ctx context.Context, environment, kind string, selector identity.Selector) (identity.LUID, error) {
	if err := batchinput.Authorize(ctx, r, environment); err != nil {
		return "", err
	}
	if kind == "user" {
		connection, err := newRemoteAdminCommands(r.runtime).connect(ctx, environment, true)
		if err != nil {
			return "", remoteSetupError(batchinput.Operation(ctx), environment, "", connection.environment, err)
		}
		user, err := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{LUID: string(selector.LUID), Username: selector.Name})
		return identity.LUID(user.LUID), batchPreparationError(ctx, environment, connection.environment.SiteContentURL, err)
	}
	connection, err := newRemoteContentCommands(r.runtime).connect(ctx, environment, true)
	if err != nil {
		return "", remoteSetupError(batchinput.Operation(ctx), environment, "", connection.environment, err)
	}
	switch kind {
	case "workbook":
		item, err := connection.workbooks.ResolveWorkbook(ctx, selector)
		return identity.LUID(item.LUID), batchPreparationError(ctx, environment, connection.environment.SiteContentURL, err)
	case "datasource":
		item, err := connection.datasources.ResolveDatasource(ctx, selector)
		return identity.LUID(item.LUID), batchPreparationError(ctx, environment, connection.environment.SiteContentURL, err)
	case "flow":
		item, err := connection.flows.ResolveFlow(ctx, selector)
		return identity.LUID(item.LUID), batchPreparationError(ctx, environment, connection.environment.SiteContentURL, err)
	case "project":
		item, err := connection.projects.ResolveProject(ctx, selector)
		return identity.LUID(item.LUID), batchPreparationError(ctx, environment, connection.environment.SiteContentURL, err)
	default:
		return "", fmt.Errorf("unsupported batch resource kind %q", kind)
	}
}

func batchPreparationError(ctx context.Context, environment, site string, cause error) error {
	if cause == nil {
		return nil
	}
	if _, ok := errors.AsType[*errs.Error](cause); ok {
		return cause
	}
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Review the exact selected target and its current availability, then retry.")
	operation := batchinput.Operation(ctx)
	return &errs.Error{ID: operation + ".batch.prepare", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site,
		Summary: "Batch target preparation failed before any mutation was attempted.", Cause: cause, Retryable: retryable, CorrectiveAction: corrective,
		TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}
