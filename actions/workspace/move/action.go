// Package move implements workspace.move.
package move

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

const maxMoveWarnings = 20

// Input selects one exact artifact and destination workspace.
type Input struct {
	SourceWorkspace      string
	DestinationWorkspace string
	Kind                 string
	LUID                 string
	Path                 string
}

// Artifact is the complete moved artifact result.
type Artifact struct {
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

// Output is the stable move result.
type Output struct {
	Status               string   `json:"status"`
	Artifact             Artifact `json:"artifact"`
	SourceWorkspace      string   `json:"source_workspace"`
	DestinationWorkspace string   `json:"destination_workspace"`
	Warnings             []string `json:"warnings,omitempty"`
	WarningsOmitted      int      `json:"warnings_omitted,omitempty"`
	Help                 []string `json:"help"`
}

type compactArtifact struct {
	Kind string `json:"kind"`
	LUID string `json:"luid"`
	Name string `json:"name,omitempty"`
	Path string `json:"path"`
}

type compactOutput struct {
	Status               string          `json:"status"`
	Artifact             compactArtifact `json:"artifact"`
	SourceWorkspace      string          `json:"source_workspace"`
	DestinationWorkspace string          `json:"destination_workspace"`
	Warnings             []string        `json:"warnings,omitempty"`
	WarningsOmitted      int             `json:"warnings_omitted,omitempty"`
	Details              string          `json:"details"`
	Help                 []string        `json:"help"`
}

// CompactOutput returns the actionable moved identity.
func (o Output) CompactOutput() any {
	return compactOutput{Status: o.Status, Artifact: compactArtifact{Kind: o.Artifact.Kind, LUID: o.Artifact.LUID, Name: o.Artifact.Name, Path: o.Artifact.Path}, SourceWorkspace: o.SourceWorkspace, DestinationWorkspace: o.DestinationWorkspace, Warnings: o.Warnings, WarningsOmitted: o.WarningsOmitted, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded source identity and fingerprints.
func (o Output) FullOutput() any { return o }

// Mover moves one exact managed artifact.
type Mover interface {
	Move(context.Context, Input) (Artifact, error)
}

// Action orchestrates workspace.move.
type Action struct{ mover Mover }

// New creates workspace.move.
func New(mover Mover) *Action { return &Action{mover: mover} }

// Execute moves one exact artifact without changing its identity.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.mover == nil {
		return Output{}, runtimeError("workspace move is not configured")
	}
	if input.SourceWorkspace == "" || input.DestinationWorkspace == "" {
		return Output{}, usage("source and destination workspace names are required")
	}
	if input.Path == "" && (input.Kind == "" || input.LUID == "") {
		return Output{}, usage("artifact kind and LUID, or exact managed path, are required")
	}
	moved, err := a.mover.Move(ctx, input)
	if err != nil {
		return Output{}, &errs.Error{ID: "workspace.move.failed", Kind: errs.KindOperation, Operation: "workspace.move", Resource: input.LUID, Summary: "Workspace artifact move failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Review both exact workspaces and the artifact identity, then retry."}
	}
	warnings := append([]string(nil), moved.Warnings...)
	if moved.State == "dirty" {
		warnings = append(warnings, "The moved artifact contains local changes relative to its pulled baseline.")
	}
	warnings, warningsOmitted := boundMoveWarnings(warnings)
	return Output{Status: "moved", Artifact: moved, SourceWorkspace: input.SourceWorkspace, DestinationWorkspace: input.DestinationWorkspace, Warnings: warnings, WarningsOmitted: warningsOmitted, Help: []string{"tadx workspace status --workspace " + input.DestinationWorkspace}}, nil
}

func boundMoveWarnings(input []string) ([]string, int) {
	unique := make([]string, 0, min(len(input), maxMoveWarnings))
	seen := make(map[string]bool, len(input))
	for _, warning := range input {
		if warning == "" || seen[warning] {
			continue
		}
		seen[warning] = true
		unique = append(unique, warning)
	}
	if len(unique) <= maxMoveWarnings {
		return unique, 0
	}
	return unique[:maxMoveWarnings], len(unique) - maxMoveWarnings
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.move.usage", Kind: errs.KindUsage, Operation: "workspace.move", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Correct the exact workspace move input and retry."}
}

func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.move.runtime", Kind: errs.KindRuntime, Operation: "workspace.move", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure workspace artifact moves before retrying."}
}
