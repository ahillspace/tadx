package workspace

import (
	"context"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// RegisterInput names an existing machine-local workspace root and an optional logical
// name override for the registry.
type RegisterInput struct {
	Preview bool
	Path    string
	Name    string
}

// RegisterOutput is the stable register result.
type RegisterOutput struct {
	Status    string       `json:"status"`
	Path      string       `json:"path,omitempty"`
	Workspace Registration `json:"workspace"`
	Help      []string     `json:"help"`
}

type registerCompactOutput struct {
	Status    string    `json:"status"`
	Workspace Workspace `json:"workspace"`
	Details   string    `json:"details"`
	Help      []string  `json:"help"`
}

// CompactOutput returns the token-bounded register result.
func (o RegisterOutput) CompactOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return registerCompactOutput{Status: o.Status, Workspace: o.Workspace.Workspace, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded workspace identity details.
func (o RegisterOutput) FullOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return o
}

func (o RegisterOutput) previewOutput() any {
	return struct {
		Status       string `json:"status"`
		Name         string `json:"name"`
		Path         string `json:"path"`
		WillRegister bool   `json:"will_register"`
	}{o.Status, o.Workspace.Name, o.Path, true}
}

// Registrar adopts one existing workspace directory into the registry.
type Registrar interface {
	Register(context.Context, RegisterInput) (Registration, error)
}

// Register adopts one existing workspace directory.
func (a *Service) Register(ctx context.Context, input RegisterInput) (RegisterOutput, error) {
	registered, err := a.Registrar.Register(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workspace collision with tadx workspace list --full; do not overwrite or re-register it automatically. Otherwise point --path at an existing workspace that has a valid tadx.yaml, or create one first with tadx workspace create.")
		return RegisterOutput{}, &errs.Error{ID: "workspace.register.failed", Kind: errs.KindOperation, Operation: "workspace.register", Resource: input.Name, Summary: "Workspace registration failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if input.Preview {
		return RegisterOutput{Status: "preview", Path: input.Path, Workspace: registered, Help: []string{"Preview only; no files or configuration changed."}}, nil
	}
	if registered.Name == "" || registered.ID == "" || registered.Root == "" || !registered.Registered {
		return RegisterOutput{}, registerRuntimeError("workspace registration returned an incomplete identity")
	}
	return RegisterOutput{Status: "registered", Workspace: registered, Help: []string{commandhint.Command("workspace", "status", "--workspace", registered.Name)}}, nil
}

func registerRuntimeError(message string) error {
	return &errs.Error{ID: "workspace.register.runtime", Kind: errs.KindRuntime, Operation: "workspace.register", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace storage before retrying."}
}
