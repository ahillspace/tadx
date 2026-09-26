package workspace

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

var cleanAdmittedClasses = map[string]bool{"temporary": true, "cache": true, "logs": true, "all": true}

// CleanInput selects one workspace and one admitted disposable-state class.
type CleanInput struct {
	Preview   bool
	Workspace string
	Class     string
}

// CleanResult describes bounded cleanup effects.
type CleanResult struct {
	Status                      string   `json:"status"`
	Workspace                   string   `json:"workspace"`
	Class                       string   `json:"class"`
	EntriesRemoved              int      `json:"entries_removed"`
	BytesRemoved                int64    `json:"bytes_removed"`
	Removed                     []string `json:"removed,omitempty"`
	CanonicalArtifactsPreserved bool     `json:"canonical_artifacts_preserved"`
}

// CleanOutput contains the cleanup receipt and next command guidance.
type CleanOutput struct {
	CleanResult
	Help []string `json:"help"`
}

type cleanCompactOutput struct {
	Status                      string   `json:"status"`
	Workspace                   string   `json:"workspace"`
	Class                       string   `json:"class"`
	EntriesRemoved              int      `json:"entries_removed"`
	BytesRemoved                int64    `json:"bytes_removed"`
	Removed                     []string `json:"removed,omitempty"`
	CanonicalArtifactsPreserved bool     `json:"canonical_artifacts_preserved"`
	Details                     string   `json:"details"`
	Help                        []string `json:"help"`
}

// CompactOutput retains the bounded paths needed to review cleanup effects.
func (o CleanOutput) CompactOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return cleanCompactOutput{Status: o.Status, Workspace: o.Workspace, Class: o.Class, EntriesRemoved: o.EntriesRemoved, BytesRemoved: o.BytesRemoved, Removed: append([]string(nil), o.Removed...), CanonicalArtifactsPreserved: o.CanonicalArtifactsPreserved, Details: "--full", Help: o.Help}
}

// FullOutput includes bounded workspace-relative removed paths.
func (o CleanOutput) FullOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return o
}

func (o CleanOutput) previewOutput() any {
	var paths []string
	paths = append(paths, o.Removed...)
	return struct {
		Status    string   `json:"status"`
		Workspace string   `json:"workspace"`
		Class     string   `json:"class"`
		Entries   int      `json:"entries_to_remove"`
		Bytes     int64    `json:"bytes_to_remove"`
		Paths     []string `json:"paths_to_remove,omitempty"`
		Preserved bool     `json:"canonical_artifacts_preserved"`
	}{o.Status, o.Workspace, o.Class, o.EntriesRemoved, o.BytesRemoved, paths, true}
}

// CleanStore removes one exact disposable class without touching managed artifacts.
type CleanStore interface {
	Clean(context.Context, CleanInput) (CleanResult, error)
}

// Clean validates and applies one local cleanup operation.
func (a *Service) Clean(ctx context.Context, input CleanInput) (CleanOutput, error) {
	if !cleanAdmittedClasses[input.Class] {
		message := "workspace cleanup requires --workspace and --class temporary, cache, logs, or all"
		return CleanOutput{}, &errs.Error{ID: "workspace.clean.usage", Kind: errs.KindUsage, Operation: "workspace.clean", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Select one exact workspace and disposable cleanup class."}
	}
	result, err := a.Cleaner.Clean(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the selected workspace state before retrying: "+commandhint.Command("workspace", "status", "--workspace", input.Workspace))
		return CleanOutput{}, &errs.Error{ID: "workspace.clean.failed", Kind: errs.KindOperation, Operation: "workspace.clean", Resource: input.Workspace, Summary: "Workspace cleanup failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	result.Status, result.Workspace, result.Class = "cleaned", input.Workspace, input.Class
	if input.Preview {
		result.Status = "preview"
	}
	result.CanonicalArtifactsPreserved = true
	return CleanOutput{CleanResult: result, Help: []string{commandhint.Command("workspace", "status", "--workspace", input.Workspace)}}, nil
}
