package workspace

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// DeleteArtifactInput selects one exact managed artifact.
type DeleteArtifactInput struct {
	Workspace string
	Kind      string
	LUID      string
	Path      string
	Force     bool
}

// ArtifactTarget is one complete managed artifact identity and state.
type ArtifactTarget struct {
	Kind                string   `json:"kind"`
	LUID                string   `json:"luid"`
	Name                string   `json:"name,omitempty"`
	Path                string   `json:"path"`
	State               string   `json:"state"`
	CanonicalPath       string   `json:"canonical_path,omitempty"`
	ServerOrigin        string   `json:"source_server_origin,omitempty"`
	SiteLUID            string   `json:"source_site_luid,omitempty"`
	BaselineFingerprint string   `json:"baseline_fingerprint,omitempty"`
	CurrentFingerprint  string   `json:"current_fingerprint,omitempty"`
	TreeFingerprint     string   `json:"-"`
	Warnings            []string `json:"-"`
}

// DeleteArtifactPlan is the deterministic read-only deletion preview.
type DeleteArtifactPlan struct {
	Mode      string         `json:"mode"`
	Operation string         `json:"operation"`
	Workspace string         `json:"workspace"`
	Artifact  ArtifactTarget `json:"artifact"`
	Dirty     bool           `json:"dirty"`
	Force     bool           `json:"force"`
}

// DeleteArtifactResult is the exact deletion result.
type DeleteArtifactResult struct {
	Status   string   `json:"status"`
	Kind     string   `json:"kind"`
	LUID     string   `json:"luid"`
	Name     string   `json:"name,omitempty"`
	Path     string   `json:"path"`
	Warnings []string `json:"-"`
}

// DeleteArtifactOutput keeps the result attached to its previewed plan.
type DeleteArtifactOutput struct {
	Plan            DeleteArtifactPlan    `json:"plan"`
	Result          *DeleteArtifactResult `json:"result,omitempty"`
	Warnings        []string              `json:"warnings,omitempty"`
	WarningsOmitted int                   `json:"warnings_omitted,omitzero"`
	Help            []string              `json:"help"`
}

// ArtifactDeleteRequest carries the exact artifact state to revalidate.
type ArtifactDeleteRequest struct {
	Workspace string
	Expected  ArtifactTarget
}

type deleteArtifactCompactArtifact struct {
	Kind  string `json:"kind"`
	LUID  string `json:"luid"`
	Name  string `json:"name,omitempty"`
	Path  string `json:"path"`
	State string `json:"state"`
}

type deleteArtifactCompactPlan struct {
	Mode      string                        `json:"mode"`
	Operation string                        `json:"operation"`
	Workspace string                        `json:"workspace"`
	Artifact  deleteArtifactCompactArtifact `json:"artifact"`
	Dirty     bool                          `json:"dirty"`
	Force     bool                          `json:"force"`
}

type deleteArtifactCompactOutput struct {
	Plan            deleteArtifactCompactPlan `json:"plan"`
	Result          *DeleteArtifactResult     `json:"result,omitempty"`
	Warnings        []string                  `json:"warnings,omitempty"`
	WarningsOmitted int                       `json:"warnings_omitted,omitzero"`
	Details         string                    `json:"details"`
	Help            []string                  `json:"help"`
}

// CompactOutput returns every field needed to authorize deletion.
func (o DeleteArtifactOutput) CompactOutput() any {
	artifact := o.Plan.Artifact
	return deleteArtifactCompactOutput{Plan: deleteArtifactCompactPlan{Mode: o.Plan.Mode, Operation: o.Plan.Operation, Workspace: o.Plan.Workspace, Artifact: deleteArtifactCompactArtifact{Kind: artifact.Kind, LUID: artifact.LUID, Name: artifact.Name, Path: artifact.Path, State: artifact.State}, Dirty: o.Plan.Dirty, Force: o.Plan.Force}, Result: o.Result, Warnings: o.Warnings, WarningsOmitted: o.WarningsOmitted, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded provenance and fingerprint details.
func (o DeleteArtifactOutput) FullOutput() any { return o }

// ArtifactStore resolves and deletes exact managed artifacts.
type ArtifactStore interface {
	ResolveArtifact(context.Context, DeleteArtifactInput) (ArtifactTarget, error)
	DeleteArtifact(context.Context, ArtifactDeleteRequest) (ArtifactTarget, error)
}

// Plan resolves the exact artifact without mutation.
func (a *Service) planDeleteArtifact(ctx context.Context, input DeleteArtifactInput) (DeleteArtifactPlan, error) {
	artifact, err := a.ArtifactStore.ResolveArtifact(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact managed artifact selector, then retry.")
		return DeleteArtifactPlan{}, &errs.Error{ID: "workspace.artifact.delete.resolve", Kind: errs.KindOperation, Operation: "workspace.artifact.delete", Resource: input.LUID, Summary: "Artifact deletion target resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if artifact.Kind == "" || artifact.LUID == "" || artifact.Path == "" || artifact.TreeFingerprint == "" || (artifact.State != "missing" && artifact.CurrentFingerprint == "") {
		return DeleteArtifactPlan{}, deleteArtifactRuntimeError("artifact deletion target has incomplete authoritative identity")
	}
	return DeleteArtifactPlan{Mode: "preview", Operation: "workspace.artifact.delete", Workspace: input.Workspace, Artifact: artifact, Dirty: artifact.State == "dirty", Force: input.Force}, nil
}

// Apply performs only an internally produced exact plan.
func (a *Service) applyDeleteArtifact(ctx context.Context, plan DeleteArtifactPlan) (DeleteArtifactResult, error) {
	if plan.Dirty && !plan.Force {
		return DeleteArtifactResult{}, &errs.Error{ID: "workspace.artifact.delete.dirty", Kind: errs.KindOperation, Operation: "workspace.artifact.delete", Resource: plan.Artifact.LUID, Summary: "Dirty artifact deletion requires explicit force.", Cause: errors.New("the canonical payload differs from its pulled baseline"), Retryable: errs.Bool(false), CorrectiveAction: "Review local changes, then add --force only when deletion is intended."}
	}
	deleted, err := a.ArtifactStore.DeleteArtifact(ctx, ArtifactDeleteRequest{Workspace: plan.Workspace, Expected: plan.Artifact})
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve the exact artifact again and review a new deletion preview.")
		return DeleteArtifactResult{}, &errs.Error{ID: "workspace.artifact.delete.failed", Kind: errs.KindOperation, Operation: "workspace.artifact.delete", Resource: plan.Artifact.LUID, Summary: "Artifact deletion failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	return DeleteArtifactResult{Status: "deleted", Kind: deleted.Kind, LUID: deleted.LUID, Name: deleted.Name, Path: deleted.Path, Warnings: append([]string(nil), deleted.Warnings...)}, nil
}

// DeleteArtifact plans every call and deletes unless preview is requested.
func (a *Service) DeleteArtifact(ctx context.Context, input DeleteArtifactInput, preview bool) (DeleteArtifactOutput, error) {
	plan, err := a.planDeleteArtifact(ctx, input)
	if err != nil {
		return DeleteArtifactOutput{}, err
	}
	output := DeleteArtifactOutput{Plan: plan, Help: []string{"Run without --preview to delete this exact artifact."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	result, err := a.applyDeleteArtifact(ctx, plan)
	if err != nil {
		return DeleteArtifactOutput{}, err
	}
	output.Result = &result
	output.Warnings, output.WarningsOmitted = boundMutationWarnings(result.Warnings)
	output.Help = []string{commandhint.Command("workspace", "status", "--workspace", input.Workspace)}
	return output, nil
}

func deleteArtifactRuntimeError(message string) error {
	return &errs.Error{ID: "workspace.artifact.delete.runtime", Kind: errs.KindRuntime, Operation: "workspace.artifact.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure managed artifact deletion before retrying."}
}
