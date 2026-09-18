// Package clean removes explicitly selected disposable workspace state.
package clean

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
)

var admittedClasses = map[string]bool{"temporary": true, "cache": true, "logs": true, "all": true}

// Input selects one workspace and one admitted disposable-state class.
type Input struct {
	Preview   bool
	Workspace string
	Class     string
}

// Request is the exact cleanup operation delegated to local storage.
type Request = Input

// Result describes bounded cleanup effects.
type Result struct {
	Status                      string   `json:"status"`
	Workspace                   string   `json:"workspace"`
	Class                       string   `json:"class"`
	EntriesRemoved              int      `json:"entries_removed"`
	BytesRemoved                int64    `json:"bytes_removed"`
	Removed                     []string `json:"removed,omitempty"`
	CanonicalArtifactsPreserved bool     `json:"canonical_artifacts_preserved"`
}

// Output contains the cleanup receipt and next command guidance.
type Output struct {
	Result
	Help []string `json:"help"`
}

type compactOutput struct {
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
func (o Output) CompactOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return compactOutput{Status: o.Status, Workspace: o.Workspace, Class: o.Class, EntriesRemoved: o.EntriesRemoved, BytesRemoved: o.BytesRemoved, Removed: append([]string(nil), o.Removed...), CanonicalArtifactsPreserved: o.CanonicalArtifactsPreserved, Details: "--full", Help: o.Help}
}

// FullOutput includes bounded workspace-relative removed paths.
func (o Output) FullOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return o
}

func (o Output) previewOutput() any {
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

// Store removes one exact disposable class without touching managed artifacts.
type Store interface {
	Clean(context.Context, Request) (Result, error)
}

// Action orchestrates workspace.clean.
type Action struct{ store Store }

// New creates a workspace cleanup action.
func New(store Store) *Action { return &Action{store: store} }

// Execute validates and applies one local cleanup operation.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.store == nil {
		return Output{}, &errs.Error{ID: "workspace.clean.unconfigured", Kind: errs.KindRuntime, Operation: "workspace.clean", Summary: "Workspace cleanup is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workspace cleanup before retrying."}
	}
	if input.Workspace == "" || !admittedClasses[input.Class] {
		message := "workspace cleanup requires --workspace and --class temporary, cache, logs, or all"
		return Output{}, &errs.Error{ID: "workspace.clean.usage", Kind: errs.KindUsage, Operation: "workspace.clean", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Select one exact workspace and disposable cleanup class."}
	}
	result, err := a.store.Clean(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the selected workspace state before retrying: "+commandhint.Command("workspace", "status", "--workspace", input.Workspace))
		return Output{}, &errs.Error{ID: "workspace.clean.failed", Kind: errs.KindOperation, Operation: "workspace.clean", Resource: input.Workspace, Summary: "Workspace cleanup failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	result.Status, result.Workspace, result.Class = "cleaned", input.Workspace, input.Class
	if input.Preview {
		result.Status = "preview"
	}
	result.CanonicalArtifactsPreserved = true
	return Output{Result: result, Help: []string{commandhint.Command("workspace", "status", "--workspace", input.Workspace)}}, nil
}
