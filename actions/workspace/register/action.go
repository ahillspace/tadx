// Package register implements workspace.register.
package register

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// Input names an existing machine-local workspace root and an optional logical
// name override for the registry.
type Input struct {
	Path string
	Name string
}

// Workspace is the complete adopted workspace result.
type Workspace struct {
	Name            string `json:"name"`
	ID              string `json:"id"`
	ManifestVersion int    `json:"manifest_version"`
	Registered      bool   `json:"registered"`
}

// Output is the stable register result.
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

// CompactOutput returns the token-bounded register result.
func (o Output) CompactOutput() any {
	return compactOutput{Status: o.Status, Workspace: compactWorkspace{Name: o.Workspace.Name}, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded workspace identity details.
func (o Output) FullOutput() any { return o }

// Registrar adopts one existing workspace directory into the registry.
type Registrar interface {
	Register(context.Context, Input) (Workspace, error)
}

// Action orchestrates workspace.register.
type Action struct{ registrar Registrar }

// New creates workspace.register.
func New(registrar Registrar) *Action { return &Action{registrar: registrar} }

// Execute adopts one existing workspace directory.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.registrar == nil {
		return Output{}, runtimeError("workspace registration is not configured")
	}
	if input.Path == "" {
		return Output{}, usage("path is required")
	}
	registered, err := a.registrar.Register(ctx, input)
	if err != nil {
		return Output{}, &errs.Error{ID: "workspace.register.failed", Kind: errs.KindOperation, Operation: "workspace.register", Resource: input.Name, Summary: "Workspace registration failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Point --path at an existing workspace that has a valid tadx.yaml, or create one first with tadx workspace create."}
	}
	if registered.Name == "" || registered.ID == "" || !registered.Registered {
		return Output{}, runtimeError("workspace registration returned an incomplete identity")
	}
	return Output{Status: "registered", Workspace: registered, Help: []string{"tadx workspace status --workspace " + registered.Name}}, nil
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.register.usage", Kind: errs.KindUsage, Operation: "workspace.register", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide the machine-local root of an existing workspace."}
}

func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.register.runtime", Kind: errs.KindRuntime, Operation: "workspace.register", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace storage before retrying."}
}
