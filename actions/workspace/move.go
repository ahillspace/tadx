package workspace

import (
	"context"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// MoveInput selects one exact artifact and destination workspace.
type MoveInput struct {
	Preview              bool
	SourceWorkspace      string
	DestinationWorkspace string
	Kind                 string
	LUID                 string
	Path                 string
}

// MoveArtifact is the complete moved artifact result.
type MoveArtifact struct {
	Kind                string   `json:"kind"`
	LUID                string   `json:"luid"`
	Name                string   `json:"name,omitempty"`
	Path                string   `json:"path"`
	OldPath             string   `json:"old_path,omitempty"`
	State               string   `json:"state"`
	ServerOrigin        string   `json:"source_server_origin,omitempty"`
	SiteLUID            string   `json:"source_site_luid,omitempty"`
	BaselineFingerprint string   `json:"baseline_fingerprint,omitempty"`
	CurrentFingerprint  string   `json:"current_fingerprint,omitempty"`
	Warnings            []string `json:"-"`
}

// MoveOutput is the stable move result.
type MoveOutput struct {
	Status               string       `json:"status"`
	Artifact             MoveArtifact `json:"artifact"`
	SourceWorkspace      string       `json:"source_workspace"`
	DestinationWorkspace string       `json:"destination_workspace"`
	Warnings             []string     `json:"warnings,omitempty"`
	WarningsOmitted      int          `json:"warnings_omitted,omitzero"`
	Help                 []string     `json:"help"`
}

type moveCompactArtifact struct {
	Kind string `json:"kind"`
	LUID string `json:"luid"`
	Name string `json:"name,omitempty"`
	Path string `json:"path"`
}

type moveCompactOutput struct {
	Status               string              `json:"status"`
	Artifact             moveCompactArtifact `json:"artifact"`
	SourceWorkspace      string              `json:"source_workspace"`
	DestinationWorkspace string              `json:"destination_workspace"`
	Warnings             []string            `json:"warnings,omitempty"`
	WarningsOmitted      int                 `json:"warnings_omitted,omitzero"`
	Details              string              `json:"details"`
	Help                 []string            `json:"help"`
}

// CompactOutput returns the actionable moved identity.
func (o MoveOutput) CompactOutput() any {
	return moveCompactOutput{Status: o.Status, Artifact: moveCompactArtifact{Kind: o.Artifact.Kind, LUID: o.Artifact.LUID, Name: o.Artifact.Name, Path: o.Artifact.Path}, SourceWorkspace: o.SourceWorkspace, DestinationWorkspace: o.DestinationWorkspace, Warnings: o.Warnings, WarningsOmitted: o.WarningsOmitted, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded source identity and fingerprints.
func (o MoveOutput) FullOutput() any { return o }

// Mover moves one exact managed artifact.
type Mover interface {
	Move(context.Context, MoveInput) (MoveArtifact, error)
}

// Move moves one exact artifact without changing its identity.
func (a *Service) Move(ctx context.Context, input MoveInput) (MoveOutput, error) {
	moved, err := a.Mover.Move(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review both exact workspaces and the artifact identity, then retry.")
		return MoveOutput{}, &errs.Error{ID: "workspace.move.failed", Kind: errs.KindOperation, Operation: "workspace.move", Resource: input.LUID, Summary: "Workspace artifact move failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if input.Preview {
		return MoveOutput{Status: "preview", Artifact: moved, SourceWorkspace: input.SourceWorkspace, DestinationWorkspace: input.DestinationWorkspace, Help: []string{"Preview only; no artifact moved."}}, nil
	}
	warnings := append([]string(nil), moved.Warnings...)
	if moved.State == "dirty" {
		warnings = append(warnings, "The moved artifact contains local changes relative to its pulled baseline.")
	}
	warnings, warningsOmitted := boundMutationWarnings(warnings)
	return MoveOutput{Status: "moved", Artifact: moved, SourceWorkspace: input.SourceWorkspace, DestinationWorkspace: input.DestinationWorkspace, Warnings: warnings, WarningsOmitted: warningsOmitted, Help: []string{commandhint.Command("workspace", "status", "--workspace", input.DestinationWorkspace)}}, nil
}
