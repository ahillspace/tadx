// Package delete implements workspace.artifact.delete.
package delete

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

const maxDeleteWarnings = 20

// Input selects one exact managed artifact.
type Input struct {
	Workspace string
	Kind      string
	LUID      string
	Path      string
	Force     bool
}

// Artifact is one complete managed artifact identity and state.
type Artifact struct {
	Kind                string   `json:"kind"`
	LUID                string   `json:"luid"`
	Name                string   `json:"name,omitempty"`
	Path                string   `json:"path"`
	State               string   `json:"state"`
	CanonicalPath       string   `json:"canonical_path,omitempty"`
	ServerOrigin        string   `json:"source_server_origin,omitempty"`
	SiteLUID            string   `json:"source_site_luid,omitempty"`
	BaselineFingerprint string   `json:"baseline_fingerprint,omitempty"`
	CurrentFingerprint  string   `json:"current_fingerprint,omitempty"`
	TreeFingerprint     string   `json:"-"`
	Warnings            []string `json:"-"`
}

// Plan is the deterministic read-only deletion preview.
type Plan struct {
	Mode      string   `json:"mode"`
	Operation string   `json:"operation"`
	Workspace string   `json:"workspace"`
	Artifact  Artifact `json:"artifact"`
	Dirty     bool     `json:"dirty"`
	Force     bool     `json:"force"`
	planned   bool
}

// Result is the exact deletion result.
type Result struct {
	Status   string   `json:"status"`
	Kind     string   `json:"kind"`
	LUID     string   `json:"luid"`
	Name     string   `json:"name,omitempty"`
	Path     string   `json:"path"`
	Warnings []string `json:"-"`
}

// Output keeps the result attached to its previewed plan.
type Output struct {
	Plan            Plan     `json:"plan"`
	Result          *Result  `json:"result,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	WarningsOmitted int      `json:"warnings_omitted,omitempty"`
	Help            []string `json:"help"`
}

// DeleteRequest carries the exact artifact state to revalidate.
type DeleteRequest struct {
	Workspace string
	Expected  Artifact
}

type compactArtifact struct {
	Kind  string `json:"kind"`
	LUID  string `json:"luid"`
	Name  string `json:"name,omitempty"`
	Path  string `json:"path"`
	State string `json:"state"`
}

type compactPlan struct {
	Mode      string          `json:"mode"`
	Operation string          `json:"operation"`
	Workspace string          `json:"workspace"`
	Artifact  compactArtifact `json:"artifact"`
	Dirty     bool            `json:"dirty"`
	Force     bool            `json:"force"`
}

type compactOutput struct {
	Plan            compactPlan `json:"plan"`
	Result          *Result     `json:"result,omitempty"`
	Warnings        []string    `json:"warnings,omitempty"`
	WarningsOmitted int         `json:"warnings_omitted,omitempty"`
	Details         string      `json:"details"`
	Help            []string    `json:"help"`
}

// CompactOutput returns every field needed to authorize deletion.
func (o Output) CompactOutput() any {
	artifact := o.Plan.Artifact
	return compactOutput{Plan: compactPlan{Mode: o.Plan.Mode, Operation: o.Plan.Operation, Workspace: o.Plan.Workspace, Artifact: compactArtifact{Kind: artifact.Kind, LUID: artifact.LUID, Name: artifact.Name, Path: artifact.Path, State: artifact.State}, Dirty: o.Plan.Dirty, Force: o.Plan.Force}, Result: o.Result, Warnings: o.Warnings, WarningsOmitted: o.WarningsOmitted, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded provenance and fingerprint details.
func (o Output) FullOutput() any { return o }

// Store resolves and deletes exact managed artifacts.
type Store interface {
	Resolve(context.Context, Input) (Artifact, error)
	Delete(context.Context, DeleteRequest) (Artifact, error)
}

// Action orchestrates workspace.artifact.delete.
type Action struct{ store Store }

// New creates workspace.artifact.delete.
func New(store Store) *Action { return &Action{store: store} }

// Plan resolves the exact artifact without mutation.
func (a *Action) Plan(ctx context.Context, input Input) (Plan, error) {
	if a == nil || a.store == nil {
		return Plan{}, runtimeError("workspace artifact deletion is not configured")
	}
	if input.Workspace == "" || (input.Path == "" && (input.Kind == "" || input.LUID == "")) {
		return Plan{}, usage("workspace and an exact artifact selector are required")
	}
	artifact, err := a.store.Resolve(ctx, input)
	if err != nil {
		return Plan{}, &errs.Error{ID: "workspace.artifact.delete.resolve", Kind: errs.KindOperation, Operation: "workspace.artifact.delete", Resource: input.LUID, Summary: "Artifact deletion target resolution failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Review the exact managed artifact selector, then retry."}
	}
	if artifact.Kind == "" || artifact.LUID == "" || artifact.Path == "" || artifact.TreeFingerprint == "" || (artifact.State != "missing" && artifact.CurrentFingerprint == "") {
		return Plan{}, runtimeError("artifact deletion target has incomplete authoritative identity")
	}
	return Plan{Mode: "preview", Operation: "workspace.artifact.delete", Workspace: input.Workspace, Artifact: artifact, Dirty: artifact.State == "dirty", Force: input.Force, planned: true}, nil
}

// Apply performs only an internally produced exact plan.
func (a *Action) Apply(ctx context.Context, plan Plan) (Result, error) {
	if a == nil || a.store == nil || !plan.planned || plan.Operation != "workspace.artifact.delete" {
		return Result{}, usage("artifact deletion apply requires a plan produced by Plan")
	}
	if plan.Dirty && !plan.Force {
		return Result{}, &errs.Error{ID: "workspace.artifact.delete.dirty", Kind: errs.KindOperation, Operation: "workspace.artifact.delete", Resource: plan.Artifact.LUID, Summary: "Dirty artifact deletion requires explicit force.", Cause: errors.New("the canonical payload differs from its pulled baseline"), Retryable: errs.Bool(false), CorrectiveAction: "Review local changes, then add --force only when deletion is intended."}
	}
	deleted, err := a.store.Delete(ctx, DeleteRequest{Workspace: plan.Workspace, Expected: plan.Artifact})
	if err != nil {
		return Result{}, &errs.Error{ID: "workspace.artifact.delete.failed", Kind: errs.KindOperation, Operation: "workspace.artifact.delete", Resource: plan.Artifact.LUID, Summary: "Artifact deletion failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the exact artifact again and review a new deletion preview."}
	}
	return Result{Status: "deleted", Kind: deleted.Kind, LUID: deleted.LUID, Name: deleted.Name, Path: deleted.Path, Warnings: append([]string(nil), deleted.Warnings...)}, nil
}

// Execute plans every call and deletes unless preview is requested.
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	plan, err := a.Plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	output := Output{Plan: plan, Help: []string{"Run without --preview to delete this exact artifact."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	result, err := a.Apply(ctx, plan)
	if err != nil {
		return Output{}, err
	}
	output.Result = &result
	output.Warnings, output.WarningsOmitted = boundDeleteWarnings(result.Warnings)
	output.Help = []string{"tadx workspace status --workspace " + input.Workspace}
	return output, nil
}

func boundDeleteWarnings(input []string) ([]string, int) {
	unique := make([]string, 0, min(len(input), maxDeleteWarnings))
	seen := make(map[string]bool, len(input))
	for _, warning := range input {
		if warning == "" || seen[warning] {
			continue
		}
		seen[warning] = true
		unique = append(unique, warning)
	}
	if len(unique) <= maxDeleteWarnings {
		return unique, 0
	}
	return unique[:maxDeleteWarnings], len(unique) - maxDeleteWarnings
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.artifact.delete.usage", Kind: errs.KindUsage, Operation: "workspace.artifact.delete", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Correct the exact artifact deletion input and review a new preview."}
}

func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.artifact.delete.runtime", Kind: errs.KindRuntime, Operation: "workspace.artifact.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure managed artifact deletion before retrying."}
}
