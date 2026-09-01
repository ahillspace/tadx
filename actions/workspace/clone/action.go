// Package clone implements workspace.clone.
package clone

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// Input names an existing source workspace and the new logical name and
// machine-local root for its copy.
type Input struct {
	Source string
	Name   string
	Path   string
}

// Workspace is the complete cloned workspace result.
type Workspace struct {
	Name            string   `json:"name"`
	ID              string   `json:"id"`
	ManifestVersion int      `json:"manifest_version"`
	Registered      bool     `json:"registered"`
	CreatedEntries  []string `json:"created_entries"`
}

// Output is the stable clone result.
type Output struct {
	Status    string    `json:"status"`
	Workspace Workspace `json:"workspace"`
	Help      []string  `json:"help"`
}

type compactWorkspace struct {
	Name string `json:"name"`
}

type compactOutput struct {
	Status    string           `json:"status"`
	Workspace compactWorkspace `json:"workspace"`
	Details   string           `json:"details"`
	Help      []string         `json:"help"`
}

// CompactOutput returns the token-bounded clone result.
func (o Output) CompactOutput() any {
	return compactOutput{Status: o.Status, Workspace: compactWorkspace{Name: o.Workspace.Name}, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded workspace identity details.
func (o Output) FullOutput() any { return o }

// Cloner copies one existing workspace to a new root under a new identity.
type Cloner interface {
	Clone(context.Context, Input) (Workspace, error)
}

// Action orchestrates workspace.clone.
type Action struct{ cloner Cloner }

// New creates workspace.clone.
func New(cloner Cloner) *Action { return &Action{cloner: cloner} }

// Execute copies one existing workspace to a new root.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.cloner == nil {
		return Output{}, runtimeError("workspace cloning is not configured")
	}
	if input.Source == "" || input.Name == "" || input.Path == "" {
		return Output{}, usage("source, name, and path are required")
	}
	cloned, err := a.cloner.Clone(ctx, input)
	if err != nil {
		return Output{}, &errs.Error{ID: "workspace.clone.failed", Kind: errs.KindOperation, Operation: "workspace.clone", Resource: input.Name, Summary: "Workspace clone failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Confirm the source workspace exists and the destination path is empty, then retry."}
	}
	if cloned.Name == "" || cloned.ID == "" || !cloned.Registered {
		return Output{}, runtimeError("workspace clone returned an incomplete identity")
	}
	if len(cloned.CreatedEntries) == 0 {
		cloned.CreatedEntries = []string{"tadx.yaml", "artifacts", ".tadx"}
	}
	return Output{Status: "cloned", Workspace: cloned, Help: []string{"tadx workspace status --workspace " + cloned.Name}}, nil
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.clone.usage", Kind: errs.KindUsage, Operation: "workspace.clone", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one existing source workspace, one new logical name, and one new creation path."}
}

func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.clone.runtime", Kind: errs.KindRuntime, Operation: "workspace.clone", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace storage before retrying."}
}
