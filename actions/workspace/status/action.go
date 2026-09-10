// Package status implements workspace.status.
package status

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
)

const maxWarnings = 20

// Input selects one named workspace and bounded artifact page.
type Input struct {
	Workspace string
	Limit     int
	Cursor    string
}

// Workspace is the resolved stable workspace identity.
type Workspace struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Root string `json:"root"`
}

type compactWorkspace struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Artifact is one full managed artifact state.
type Artifact struct {
	Kind                string `json:"kind"`
	LUID                string `json:"luid"`
	Name                string `json:"name"`
	Path                string `json:"path,omitempty"`
	State               string `json:"state"`
	CanonicalPath       string `json:"canonical_path,omitempty"`
	BaselineFingerprint string `json:"baseline_fingerprint,omitempty"`
	CurrentFingerprint  string `json:"current_fingerprint,omitempty"`
}

// Inventory is one bounded artifact page and its workspace-wide state counts.
type Inventory struct {
	Returned      int        `json:"returned"`
	Total         int        `json:"total,omitempty"`
	Limit         int        `json:"limit"`
	NextCursor    string     `json:"-"`
	MoreAvailable bool       `json:"more_available"`
	ScanComplete  bool       `json:"scan_complete"`
	Clean         int        `json:"clean"`
	Dirty         int        `json:"dirty"`
	Missing       int        `json:"missing"`
	Invalid       int        `json:"invalid"`
	Items         []Artifact `json:"artifacts,omitempty"`
	Warnings      []string   `json:"-"`
}

// Output is the stable status result.
type Output struct {
	Status    string    `json:"status"`
	Workspace Workspace `json:"workspace"`
	Inventory Inventory `json:"artifacts"`
	Warnings  []string  `json:"warnings,omitempty"`
	Help      []string  `json:"help"`
}

type compactOutput struct {
	Status          string           `json:"status"`
	Workspace       compactWorkspace `json:"workspace"`
	Inventory       Inventory        `json:"artifacts"`
	Warnings        []string         `json:"warnings,omitempty"`
	WarningsOmitted int              `json:"warnings_omitted,omitempty"`
	Details         string           `json:"details"`
	Help            []string         `json:"help"`
}

type fullOutput struct {
	Status          string    `json:"status"`
	Workspace       Workspace `json:"workspace"`
	Inventory       Inventory `json:"artifacts"`
	Warnings        []string  `json:"warnings,omitempty"`
	WarningsOmitted int       `json:"warnings_omitted,omitempty"`
	Help            []string  `json:"help"`
}

// CompactOutput returns counts without artifact details.
func (o Output) CompactOutput() any {
	inventory := o.Inventory
	inventory.MoreAvailable = inventory.MoreAvailable || inventory.NextCursor != ""
	inventory.Items = make([]Artifact, len(o.Inventory.Items))
	for i, item := range o.Inventory.Items {
		inventory.Items[i] = Artifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, State: item.State}
	}
	warnings, omitted := boundWarnings(o.Warnings)
	workspace := compactWorkspace{Name: o.Workspace.Name, ID: o.Workspace.ID}
	return compactOutput{Status: o.Status, Workspace: workspace, Inventory: inventory, Warnings: warnings, WarningsOmitted: omitted, Details: "--full", Help: o.Help}
}

// FullOutput returns the same bounded page with artifact details.
func (o Output) FullOutput() any {
	warnings, omitted := boundWarnings(o.Warnings)
	inventory := o.Inventory
	inventory.MoreAvailable = inventory.MoreAvailable || inventory.NextCursor != ""
	return fullOutput{Status: o.Status, Workspace: o.Workspace, Inventory: inventory, Warnings: warnings, WarningsOmitted: omitted, Help: o.Help}
}

// Reader reports one named workspace's bounded status.
type Reader interface {
	Status(context.Context, Input) (Workspace, Inventory, error)
}

// Action orchestrates workspace.status.
type Action struct{ reader Reader }

// New creates workspace.status.
func New(reader Reader) *Action { return &Action{reader: reader} }

// Execute reports one bounded status page.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, runtimeError("workspace status is not configured")
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit < 1 || input.Limit > 10000 {
		return Output{}, usage("limit must be between 1 and 10000")
	}
	resolved, inventory, err := a.reader.Status(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair the exact workspace or artifact metadata, then retry.")
		return Output{}, &errs.Error{ID: "workspace.status.failed", Kind: errs.KindOperation, Operation: "workspace.status", Resource: input.Workspace, Summary: "Workspace status failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if inventory.Returned != len(inventory.Items) || inventory.Returned > input.Limit {
		return Output{}, runtimeError("workspace status returned an invalid bounded page")
	}
	state := "ready"
	if inventory.Dirty > 0 || inventory.Missing > 0 || inventory.Invalid > 0 || !inventory.ScanComplete {
		state = "attention"
	}
	return Output{Status: state, Workspace: resolved, Inventory: inventory, Warnings: inventory.Warnings, Help: []string{commandhint.Command("workspace", "status", "--workspace", resolved.Name, "--full")}}, nil
}

func boundWarnings(input []string) ([]string, int) {
	limit := len(input)
	omitted := 0
	if limit > maxWarnings {
		omitted = limit - maxWarnings
		limit = maxWarnings
	}
	// Copy into a fresh slice rather than reslicing the caller's backing array,
	// matching the other warning bounders so a later append by the caller cannot
	// mutate the returned view.
	bounded := make([]string, limit)
	copy(bounded, input[:limit])
	return bounded, omitted
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.status.usage", Kind: errs.KindUsage, Operation: "workspace.status", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Correct the workspace status input and retry."}
}

func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.status.runtime", Kind: errs.KindRuntime, Operation: "workspace.status", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace status before retrying."}
}
