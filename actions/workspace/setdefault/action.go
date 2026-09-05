// Package setdefault implements workspace.set-default.
package setdefault

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct{ Name string }

type Workspace struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Root string `json:"root"`
}

type Output struct {
	Status    string    `json:"status"`
	Workspace Workspace `json:"workspace"`
	Help      []string  `json:"help"`
}

func (o Output) CompactOutput() any {
	return struct {
		Status    string   `json:"status"`
		Workspace string   `json:"workspace"`
		Details   string   `json:"details"`
		Help      []string `json:"help"`
	}{o.Status, o.Workspace.Name, "--full", o.Help}
}
func (o Output) FullOutput() any { return o }

type Setter interface {
	SetDefault(context.Context, string) (Workspace, error)
}
type Action struct{ setter Setter }

func New(setter Setter) *Action { return &Action{setter: setter} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if input.Name == "" {
		return Output{}, usage("workspace name is required")
	}
	if a == nil || a.setter == nil {
		return Output{}, runtimeError("workspace default selection is not configured")
	}
	item, err := a.setter.SetDefault(ctx, input.Name)
	if err != nil {
		return Output{}, &errs.Error{ID: "workspace.set-default.failed", Kind: errs.KindOperation, Operation: "workspace.set-default", Resource: input.Name, Summary: "Workspace default selection failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Select an available registered workspace, then retry."}
	}
	if item.Name == "" || item.ID == "" || item.Root == "" {
		return Output{}, runtimeError("workspace default selection returned an incomplete identity")
	}
	return Output{Status: "default-set", Workspace: item, Help: []string{"tadx workspace status --workspace " + item.Name}}, nil
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.set-default.usage", Kind: errs.KindUsage, Operation: "workspace.set-default", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one registered workspace name."}
}
func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.set-default.runtime", Kind: errs.KindRuntime, Operation: "workspace.set-default", Summary: message, Retryable: errs.Bool(false)}
}
