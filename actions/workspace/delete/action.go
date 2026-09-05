// Package delete implements workspace.delete.
package delete

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct {
	Name  string
	Force bool
}
type Workspace struct {
	Name             string `json:"name"`
	ID               string `json:"id"`
	Root             string `json:"root"`
	Dirty            bool   `json:"dirty"`
	DirtyArtifacts   int    `json:"dirty_artifacts,omitempty"`
	InvalidArtifacts int    `json:"invalid_artifacts,omitempty"`
}
type Plan struct {
	Mode      string    `json:"mode"`
	Operation string    `json:"operation"`
	Workspace Workspace `json:"workspace"`
	Force     bool      `json:"force"`
	planned   bool
}
type Result struct {
	Status    string `json:"status"`
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type DeleteRequest struct {
	Expected Workspace
	Force    bool
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }

type Store interface {
	Resolve(context.Context, string) (Workspace, error)
	Delete(context.Context, DeleteRequest) error
}
type Action struct{ store Store }

func New(store Store) *Action { return &Action{store: store} }
func (a *Action) Plan(ctx context.Context, input Input) (Plan, error) {
	if input.Name == "" {
		return Plan{}, usage("workspace name is required")
	}
	if a == nil || a.store == nil {
		return Plan{}, runtimeError("workspace deletion is not configured")
	}
	item, err := a.store.Resolve(ctx, input.Name)
	if err != nil {
		return Plan{}, &errs.Error{ID: "workspace.delete.resolve", Kind: errs.KindOperation, Operation: "workspace.delete", Resource: input.Name, Summary: "Workspace deletion target resolution failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Review the exact registered workspace name, then retry."}
	}
	if item.Name == "" || item.ID == "" || item.Root == "" {
		return Plan{}, runtimeError("workspace deletion target has an incomplete identity")
	}
	return Plan{Mode: "preview", Operation: "workspace.delete", Workspace: item, Force: input.Force, planned: true}, nil
}
func (a *Action) Apply(ctx context.Context, plan Plan) (Result, error) {
	if a == nil || a.store == nil || !plan.planned || plan.Operation != "workspace.delete" {
		return Result{}, usage("workspace deletion requires a plan produced by Plan")
	}
	if plan.Workspace.Dirty && !plan.Force {
		return Result{}, &errs.Error{ID: "workspace.delete.dirty", Kind: errs.KindOperation, Operation: "workspace.delete", Resource: plan.Workspace.ID, Summary: "Dirty workspace deletion requires explicit force.", Cause: errors.New("the workspace contains dirty or invalid managed artifacts"), Retryable: errs.Bool(false), CorrectiveAction: "Review local changes, then add --force only when deletion is intended."}
	}
	if err := a.store.Delete(ctx, DeleteRequest{Expected: plan.Workspace, Force: plan.Force}); err != nil {
		return Result{}, &errs.Error{ID: "workspace.delete.failed", Kind: errs.KindOperation, Operation: "workspace.delete", Resource: plan.Workspace.ID, Summary: "Workspace deletion failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the exact workspace again and review a new deletion preview."}
	}
	return Result{Status: "deleted", Workspace: plan.Workspace.Name, ID: plan.Workspace.ID}, nil
}
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	plan, err := a.Plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: plan, Help: []string{"Run without --preview to delete this exact workspace."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	result, err := a.Apply(ctx, plan)
	if err != nil {
		return Output{}, err
	}
	out.Result = &result
	out.Help = []string{"tadx workspace list"}
	return out, nil
}
func usage(message string) error {
	return &errs.Error{ID: "workspace.delete.usage", Kind: errs.KindUsage, Operation: "workspace.delete", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact registered workspace name."}
}
func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.delete.runtime", Kind: errs.KindRuntime, Operation: "workspace.delete", Summary: message, Retryable: errs.Bool(false)}
}
