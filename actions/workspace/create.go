package workspace

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// CreateInput names one workspace and an optional machine-local root override.
type CreateInput struct {
	Preview bool
	Name    string
	Path    string
}

// CreateOutput is the stable create result.
type CreateOutput struct {
	Status    string           `json:"status"`
	Workspace CreatedWorkspace `json:"workspace"`
	Help      []string         `json:"help"`
}

type createCompactOutput struct {
	Status    string                   `json:"status"`
	Workspace compactWorkspaceIdentity `json:"workspace"`
	Details   string                   `json:"details"`
	Help      []string                 `json:"help"`
}

// CompactOutput returns the token-bounded create result.
func (o CreateOutput) CompactOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return createCompactOutput{Status: o.Status, Workspace: compactWorkspaceIdentity{Name: o.Workspace.Name, ID: o.Workspace.ID}, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded workspace identity details.
func (o CreateOutput) FullOutput() any {
	if o.Status == "preview" {
		return o.previewOutput()
	}
	return o
}

func (o CreateOutput) previewOutput() any {
	return struct {
		Status       string   `json:"status"`
		Name         string   `json:"name"`
		Root         string   `json:"root"`
		WillRegister bool     `json:"will_register"`
		Entries      []string `json:"planned_entries"`
	}{o.Status, o.Workspace.Name, o.Workspace.Root, true, []string{"tadx.yaml", "artifacts", ".tadx"}}
}

// Creator creates one named workspace.
type Creator interface {
	Create(context.Context, CreateInput) (Registration, error)
}

// Create creates one named workspace.
func (a *Service) Create(ctx context.Context, input CreateInput) (CreateOutput, error) {
	if input.Name == "" {
		return CreateOutput{}, createUsage("name is required")
	}
	created, err := a.Creator.Create(ctx, input)
	if configurationInstalled(err) {
		return CreateOutput{}, installedConfigurationError("workspace.create.failed", "workspace.create", created.ID, "The workspace was created and registered, but the configuration could not be made durable.", err)
	}
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workspace collision with tadx workspace list --full; do not overwrite or re-register it automatically. Otherwise review the exact workspace name and root, then retry.")
		return CreateOutput{}, &errs.Error{ID: "workspace.create.failed", Kind: errs.KindOperation, Operation: "workspace.create", Resource: input.Name, Summary: "Workspace creation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if input.Preview {
		return CreateOutput{Status: "preview", Workspace: createdWorkspace(created), Help: []string{"Preview only; no files or configuration changed."}}, nil
	}
	if created.Name == "" || created.ID == "" || created.Root == "" || !created.Registered {
		return CreateOutput{}, createRuntimeError("workspace creation returned an incomplete identity")
	}
	return CreateOutput{Status: "created", Workspace: createdWorkspace(created), Help: []string{commandhint.Command("workspace", "status", "--workspace", created.Name)}}, nil
}

// configurationInstalled reports a failure after the registry change took
// effect, so the workspace change is confirmed even though the update failed.
func configurationInstalled(err error) bool {
	var installed interface{ ConfigurationInstalled() bool }
	return errors.As(err, &installed) && installed.ConfigurationInstalled()
}

func installedConfigurationError(id, operation, resource, summary string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair access to the configuration directory, then confirm the workspace with tadx workspace list --full.")
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: operation, Resource: resource, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, Phase: errs.PhasePersistence, Outcome: errs.OutcomeConfirmed}
}

func createUsage(message string) error {
	return &errs.Error{ID: "workspace.create.usage", Kind: errs.KindUsage, Operation: "workspace.create", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one logical workspace name. Use --path only to override the default location."}
}

func createRuntimeError(message string) error {
	return &errs.Error{ID: "workspace.create.runtime", Kind: errs.KindRuntime, Operation: "workspace.create", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace storage before retrying."}
}
