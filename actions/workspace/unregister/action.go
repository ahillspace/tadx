// Package unregister implements workspace.unregister.
package unregister

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct {
	Name    string
	Preview bool
}
type Workspace struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Root string `json:"root"`
}
type Output struct {
	Status         string    `json:"status"`
	Workspace      Workspace `json:"workspace"`
	FilesPreserved bool      `json:"files_preserved"`
	Help           []string  `json:"help"`
}

func (o Output) CompactOutput() any {
	return struct {
		Status         string   `json:"status"`
		Workspace      string   `json:"workspace"`
		FilesPreserved bool     `json:"files_preserved"`
		Details        string   `json:"details"`
		Help           []string `json:"help"`
	}{o.Status, o.Workspace.Name, o.FilesPreserved, "--full", o.Help}
}
func (o Output) FullOutput() any { return o }

type Registry interface {
	Unregister(context.Context, string) (Workspace, error)
}
type Action struct{ registry Registry }

func New(registry Registry) *Action { return &Action{registry: registry} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if input.Name == "" {
		return Output{}, usage("workspace name is required")
	}
	if a == nil || a.registry == nil {
		return Output{}, runtimeError("workspace unregister is not configured")
	}
	operation := a.registry.Unregister
	if input.Preview {
		planner, ok := a.registry.(interface {
			PreviewUnregister(context.Context, string) (Workspace, error)
		})
		if !ok {
			return Output{}, runtimeError("workspace preview is not configured")
		}
		operation = planner.PreviewUnregister
	}
	item, err := operation(ctx, input.Name)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact registered workspace name, then retry.")
		return Output{}, &errs.Error{ID: "workspace.unregister.failed", Kind: errs.KindOperation, Operation: "workspace.unregister", Resource: input.Name, Summary: "Workspace unregister failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if item.Name == "" || item.ID == "" || item.Root == "" {
		return Output{}, runtimeError("workspace unregister returned an incomplete identity")
	}
	status := "unregistered"
	if input.Preview {
		status = "preview"
	}
	return Output{Status: status, Workspace: item, FilesPreserved: true, Help: []string{commandhint.Command("workspace", "register", item.Name, "--path", "<path>")}}, nil
}
func usage(message string) error {
	return &errs.Error{ID: "workspace.unregister.usage", Kind: errs.KindUsage, Operation: "workspace.unregister", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one registered workspace name."}
}
func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.unregister.runtime", Kind: errs.KindRuntime, Operation: "workspace.unregister", Summary: message, Retryable: errs.Bool(false)}
}
