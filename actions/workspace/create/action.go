// Package create implements workspace.create.
package create

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// Input names one workspace and an optional machine-local root override.
type Input struct {
	Name string
	Path string
}

// Workspace is the complete created workspace result.
type Workspace struct {
	Name            string   `json:"name"`
	ID              string   `json:"id"`
	Root            string   `json:"root"`
	ManifestVersion int      `json:"manifest_version"`
	Registered      bool     `json:"registered"`
	CreatedEntries  []string `json:"created_entries"`
}

// Output is the stable create result.
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

// CompactOutput returns the token-bounded create result.
func (o Output) CompactOutput() any {
	return compactOutput{Status: o.Status, Workspace: compactWorkspace{Name: o.Workspace.Name}, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded workspace identity details.
func (o Output) FullOutput() any { return o }

// Creator creates one named workspace.
type Creator interface {
	Create(context.Context, Input) (Workspace, error)
}

// Action orchestrates workspace.create.
type Action struct{ creator Creator }

// New creates workspace.create.
func New(creator Creator) *Action { return &Action{creator: creator} }

// Execute creates one named workspace.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.creator == nil {
		return Output{}, runtimeError("workspace creation is not configured")
	}
	if input.Name == "" {
		return Output{}, usage("name is required")
	}
	created, err := a.creator.Create(ctx, input)
	if err != nil {
		return Output{}, &errs.Error{ID: "workspace.create.failed", Kind: errs.KindOperation, Operation: "workspace.create", Resource: input.Name, Summary: "Workspace creation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Review the exact workspace name and root, then retry."}
	}
	if created.Name == "" || created.ID == "" || created.Root == "" || !created.Registered {
		return Output{}, runtimeError("workspace creation returned an incomplete identity")
	}
	if len(created.CreatedEntries) == 0 {
		created.CreatedEntries = []string{"tadx.yaml", "artifacts", ".tadx"}
	}
	return Output{Status: "created", Workspace: created, Help: []string{"tadx workspace status --workspace " + created.Name}}, nil
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.create.usage", Kind: errs.KindUsage, Operation: "workspace.create", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one logical workspace name. Use --path only to override the default location."}
}

func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.create.runtime", Kind: errs.KindRuntime, Operation: "workspace.create", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace storage before retrying."}
}
