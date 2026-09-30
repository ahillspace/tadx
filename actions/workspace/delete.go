package workspace

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

type DeleteWorkspaceInput struct {
	Name  string
	Force bool
}
type DeletionTarget struct {
	Workspace

	Dirty            bool `json:"dirty"`
	DirtyArtifacts   int  `json:"dirty_artifacts,omitzero"`
	InvalidArtifacts int  `json:"invalid_artifacts,omitzero"`
}
type DeleteWorkspacePlan struct {
	Mode      string         `json:"mode"`
	Operation string         `json:"operation"`
	Workspace DeletionTarget `json:"workspace"`
	Force     bool           `json:"force"`
}
type DeleteWorkspaceResult struct {
	Status    string `json:"status"`
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
}
type DeleteWorkspaceOutput struct {
	Plan   DeleteWorkspacePlan    `json:"plan"`
	Result *DeleteWorkspaceResult `json:"result,omitempty"`
	Help   []string               `json:"help"`
}
type WorkspaceDeleteRequest struct {
	Expected DeletionTarget
	Force    bool
}

func (o DeleteWorkspaceOutput) CompactOutput() any { return o }
func (o DeleteWorkspaceOutput) FullOutput() any    { return o }

type WorkspaceStore interface {
	ResolveWorkspace(context.Context, string) (DeletionTarget, error)
	DeleteWorkspace(context.Context, WorkspaceDeleteRequest) error
}

func (a *Service) planDeleteWorkspace(ctx context.Context, input DeleteWorkspaceInput) (DeleteWorkspacePlan, error) {
	if input.Name == "" {
		return DeleteWorkspacePlan{}, deleteWorkspaceUsage("workspace name is required")
	}
	item, err := a.WorkspaceStore.ResolveWorkspace(ctx, input.Name)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact registered workspace name, then retry.")
		return DeleteWorkspacePlan{}, &errs.Error{ID: "workspace.delete.resolve", Kind: errs.KindOperation, Operation: "workspace.delete", Resource: input.Name, Summary: "Workspace deletion target resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if item.Name == "" || item.ID == "" || item.Root == "" {
		return DeleteWorkspacePlan{}, deleteWorkspaceRuntimeError("workspace deletion target has an incomplete identity")
	}
	return DeleteWorkspacePlan{Mode: "preview", Operation: "workspace.delete", Workspace: item, Force: input.Force}, nil
}
func (a *Service) applyDeleteWorkspace(ctx context.Context, plan DeleteWorkspacePlan) (DeleteWorkspaceResult, error) {
	if plan.Workspace.Dirty && !plan.Force {
		return DeleteWorkspaceResult{}, &errs.Error{ID: "workspace.delete.dirty", Kind: errs.KindOperation, Operation: "workspace.delete", Resource: plan.Workspace.ID, Summary: "Dirty workspace deletion requires explicit force.", Cause: errors.New("the workspace contains dirty or invalid managed artifacts"), Retryable: errs.Bool(false), CorrectiveAction: "Review local changes, then add --force only when deletion is intended."}
	}
	err := a.WorkspaceStore.DeleteWorkspace(ctx, WorkspaceDeleteRequest{Expected: plan.Workspace, Force: plan.Force})
	if configurationInstalled(err) {
		return DeleteWorkspaceResult{}, installedConfigurationError("workspace.delete.failed", "workspace.delete", plan.Workspace.ID, "The workspace was unregistered, but the configuration could not be made durable.", err)
	}
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve the exact workspace again and review a new deletion preview.")
		return DeleteWorkspaceResult{}, &errs.Error{ID: "workspace.delete.failed", Kind: errs.KindOperation, Operation: "workspace.delete", Resource: plan.Workspace.ID, Summary: "Workspace deletion failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	return DeleteWorkspaceResult{Status: "deleted", Workspace: plan.Workspace.Name, ID: plan.Workspace.ID}, nil
}
func (a *Service) DeleteWorkspace(ctx context.Context, input DeleteWorkspaceInput, preview bool) (DeleteWorkspaceOutput, error) {
	plan, err := a.planDeleteWorkspace(ctx, input)
	if err != nil {
		return DeleteWorkspaceOutput{}, err
	}
	out := DeleteWorkspaceOutput{Plan: plan, Help: []string{"Run without --preview to delete this exact workspace."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	result, err := a.applyDeleteWorkspace(ctx, plan)
	if err != nil {
		return DeleteWorkspaceOutput{}, err
	}
	out.Result = &result
	out.Help = []string{"tadx workspace list"}
	return out, nil
}
func deleteWorkspaceUsage(message string) error {
	return &errs.Error{ID: "workspace.delete.usage", Kind: errs.KindUsage, Operation: "workspace.delete", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact registered workspace name."}
}
func deleteWorkspaceRuntimeError(message string) error {
	return &errs.Error{ID: "workspace.delete.runtime", Kind: errs.KindRuntime, Operation: "workspace.delete", Summary: message, Retryable: errs.Bool(false)}
}
