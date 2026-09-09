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
	Workspace string
	Class     string
}

// Request is the exact cleanup operation delegated to local storage.
type Request = Input

// Result describes bounded cleanup effects.
type Result struct {
	Status         string   `json:"status"`
	Workspace      string   `json:"workspace"`
	Class          string   `json:"class"`
	EntriesRemoved int      `json:"entries_removed"`
	BytesRemoved   int64    `json:"bytes_removed"`
	Removed        []string `json:"removed,omitempty"`
}

// Output contains the cleanup receipt and next command guidance.
type Output struct {
	Result
	Help []string `json:"help"`
}

type compactOutput struct {
	Status         string   `json:"status"`
	Workspace      string   `json:"workspace"`
	Class          string   `json:"class"`
	EntriesRemoved int      `json:"entries_removed"`
	BytesRemoved   int64    `json:"bytes_removed"`
	Details        string   `json:"details"`
	Help           []string `json:"help"`
}

// CompactOutput omits individual removed paths.
func (o Output) CompactOutput() any {
	return compactOutput{Status: o.Status, Workspace: o.Workspace, Class: o.Class, EntriesRemoved: o.EntriesRemoved, BytesRemoved: o.BytesRemoved, Details: "--full", Help: o.Help}
}

// FullOutput includes bounded workspace-relative removed paths.
func (o Output) FullOutput() any { return o }

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
		return Output{}, &errs.Error{ID: "workspace.clean.failed", Kind: errs.KindOperation, Operation: "workspace.clean", Resource: input.Workspace, Summary: "Workspace cleanup failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the selected workspace state before retrying: " + commandhint.Command("workspace", "status", "--workspace", input.Workspace)}
	}
	result.Status, result.Workspace, result.Class = "cleaned", input.Workspace, input.Class
	return Output{Result: result, Help: []string{commandhint.Command("workspace", "status", "--workspace", input.Workspace)}}, nil
}
