package workspace

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

const statusMaxWarnings = 20

// StatusMaxLimit is the largest permitted workspace artifact page.
const StatusMaxLimit = 10000

// StatusInput selects one named workspace and bounded artifact page.
type StatusInput struct {
	All       bool
	Workspace string
	Limit     int
	Cursor    string
}

// StatusArtifact is one full managed artifact state.
type StatusArtifact struct {
	Kind                string `json:"kind"`
	LUID                string `json:"luid"`
	Name                string `json:"name"`
	Path                string `json:"path,omitempty"`
	State               string `json:"state"`
	Reason              string `json:"reason,omitempty"`
	Diagnostic          string `json:"diagnostic,omitempty"`
	CanonicalPath       string `json:"canonical_path,omitempty"`
	BaselineFingerprint string `json:"baseline_fingerprint,omitempty"`
	CurrentFingerprint  string `json:"current_fingerprint,omitempty"`
}

// StatusInventory is one bounded artifact page and its workspace-wide state counts.
type StatusInventory struct {
	Returned      int              `json:"returned"`
	Total         int              `json:"total"`
	Limit         int              `json:"limit"`
	NextCursor    string           `json:"-"`
	MoreAvailable bool             `json:"more_available"`
	ScanComplete  bool             `json:"scan_complete"`
	Clean         int              `json:"clean"`
	Dirty         int              `json:"dirty"`
	Missing       int              `json:"missing"`
	Invalid       int              `json:"invalid"`
	Items         []StatusArtifact `json:"artifacts"`
	Warnings      []string         `json:"-"`
}

// StatusOutput is the stable status result.
type StatusOutput struct {
	Status    string          `json:"status"`
	Workspace Workspace       `json:"workspace"`
	Inventory StatusInventory `json:"artifacts"`
	Warnings  []string        `json:"warnings,omitempty"`
	Help      []string        `json:"help"`
}

type statusCompactOutput struct {
	Status          string                   `json:"status"`
	Workspace       compactWorkspaceIdentity `json:"workspace"`
	Inventory       StatusInventory          `json:"artifacts"`
	Warnings        []string                 `json:"warnings,omitempty"`
	WarningsOmitted int                      `json:"warnings_omitted,omitzero"`
	Details         string                   `json:"details"`
	Help            []string                 `json:"help"`
}

type statusFullOutput struct {
	Status          string          `json:"status"`
	Workspace       Workspace       `json:"workspace"`
	Inventory       StatusInventory `json:"artifacts"`
	Warnings        []string        `json:"warnings,omitempty"`
	WarningsOmitted int             `json:"warnings_omitted,omitzero"`
	Help            []string        `json:"help"`
}

// CompactOutput returns counts without artifact details.
func (o StatusOutput) CompactOutput() any {
	inventory := o.Inventory
	inventory.MoreAvailable = inventory.MoreAvailable || inventory.NextCursor != ""
	inventory.Items = make([]StatusArtifact, len(o.Inventory.Items))
	for i, item := range o.Inventory.Items {
		inventory.Items[i] = StatusArtifact{Kind: item.Kind, LUID: item.LUID, Name: item.Name, Path: item.Path, State: item.State}
	}
	warnings, omitted := statusBoundWarnings(o.Warnings)
	workspace := compactWorkspaceIdentity{Name: o.Workspace.Name, ID: o.Workspace.ID}
	return statusCompactOutput{Status: o.Status, Workspace: workspace, Inventory: inventory, Warnings: warnings, WarningsOmitted: omitted, Details: "--full", Help: o.Help}
}

// FullOutput returns the same bounded page with artifact details.
func (o StatusOutput) FullOutput() any {
	warnings, omitted := statusBoundWarnings(o.Warnings)
	inventory := o.Inventory
	inventory.MoreAvailable = inventory.MoreAvailable || inventory.NextCursor != ""
	return statusFullOutput{Status: o.Status, Workspace: o.Workspace, Inventory: inventory, Warnings: warnings, WarningsOmitted: omitted, Help: o.Help}
}

// StatusReader reports one named workspace's bounded status.
type StatusReader interface {
	Status(context.Context, StatusInput) (Workspace, StatusInventory, error)
}

// Status reports one bounded status page.
func (a *Service) Status(ctx context.Context, input StatusInput) (StatusOutput, error) {
	if input.All {
		input.Limit = StatusMaxLimit
	} else if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit < 1 || input.Limit > StatusMaxLimit {
		return StatusOutput{}, statusUsage("limit must be between 1 and 10000")
	}
	resolved, inventory, err := a.Reader.Status(ctx, input)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair the exact workspace or artifact metadata, then retry.")
		return StatusOutput{}, &errs.Error{ID: "workspace.status.failed", Kind: errs.KindOperation, Operation: "workspace.status", Resource: input.Workspace, Summary: "Workspace status failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if inventory.Returned != len(inventory.Items) || inventory.Returned > input.Limit {
		return StatusOutput{}, statusRuntimeError("workspace status returned an invalid bounded page")
	}
	if input.All {
		if inventory.Total > StatusMaxLimit {
			return StatusOutput{}, statusUsage("--all exceeds the 10000-record bound")
		}
		if inventory.Returned != inventory.Total || inventory.NextCursor != "" || !inventory.ScanComplete {
			return StatusOutput{}, statusRuntimeError("workspace status --all returned an incomplete page")
		}
	}
	state := "ready"
	if inventory.Dirty > 0 || inventory.Missing > 0 || inventory.Invalid > 0 || !inventory.ScanComplete {
		state = "attention"
	}
	return StatusOutput{Status: state, Workspace: resolved, Inventory: inventory, Warnings: inventory.Warnings, Help: []string{commandhint.Command("workspace", "status", "--workspace", resolved.Name, "--full")}}, nil
}

func statusBoundWarnings(input []string) ([]string, int) {
	limit := len(input)
	omitted := 0
	if limit > statusMaxWarnings {
		omitted = limit - statusMaxWarnings
		limit = statusMaxWarnings
	}
	// Copy into a fresh slice rather than reslicing the caller's backing array,
	// matching the other warning bounders so a later append by the caller cannot
	// mutate the returned view.
	bounded := make([]string, limit)
	copy(bounded, input[:limit])
	return bounded, omitted
}

func statusUsage(message string) error {
	return &errs.Error{ID: "workspace.status.usage", Kind: errs.KindUsage, Operation: "workspace.status", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Correct the workspace status input and retry."}
}

func statusRuntimeError(message string) error {
	return &errs.Error{ID: "workspace.status.runtime", Kind: errs.KindRuntime, Operation: "workspace.status", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace status before retrying."}
}
