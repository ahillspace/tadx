// Package get implements exact capability discovery.
package get

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/errs"
)

var (
	// ErrIDRequired identifies a missing registry ID.
	ErrIDRequired = errors.New("capability ID is required")
	// ErrNotFound identifies an unknown exact registry ID.
	ErrNotFound = errors.New("capability not found")
)

// Source resolves an exact registry ID.
type Source interface {
	Get(context.Context, string) (Capability, bool)
}

// Action gets one capability without depending on CLI plumbing.
type Action struct {
	source Source
}

// New constructs a capability get action.
func New(source Source) *Action {
	return &Action{source: source}
}

// Execute returns one exact capability definition.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.source == nil {
		return Output{}, &errs.Error{ID: "capability.get.unconfigured", Kind: errs.KindRuntime, Operation: "capability.get", Summary: "Capability get is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure a capability source before retrying."}
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return Output{}, &errs.Error{
			ID:         "capability.get.usage",
			Kind:       errs.KindUsage,
			Operation:  "capability.get",
			Summary:    ErrIDRequired.Error(),
			Cause:      ErrIDRequired,
			Validation: []errs.ValidationDetail{{Field: "id", Code: "required", Message: ErrIDRequired.Error()}},
		}
	}
	item, ok := a.source.Get(ctx, id)
	if !ok {
		return Output{}, &errs.Error{
			ID:               "capability.get.not_found",
			Kind:             errs.KindOperation,
			Operation:        "capability.get",
			Selector:         id,
			Summary:          fmt.Sprintf("capability %q was not found", id),
			Cause:            ErrNotFound,
			Retryable:        errs.Bool(false),
			CorrectiveAction: "Run tadx capability list and use an exact capability ID.",
		}
	}
	item.ExecutionEnabled = !item.PolicyDenied && item.ImplementationState == "implemented" && (!item.RemoteMutation || input.MutationsEnabled)
	return Output{Capability: item, Help: []string{"tadx capability list"}}, nil
}

// Input selects one capability by its exact registry ID.
type Input struct {
	ID               string `json:"id"`
	MutationsEnabled bool   `json:"-"`
}

// Capability is the detailed discovery view of one registry entry.
//
// The neutral representation is shared with capability.list full output so
// that the two commands cannot drift in their contract fields.
type Capability = capability.Discovery

// Output is the stable capability detail result.
type Output struct {
	Capability Capability `json:"capability"`
	Help       []string   `json:"help"`
}

type visibleOutput Output

func (o Output) CompactOutput() any {
	if o.Capability.Disposition == "delegated" {
		o.Capability.Disposition = "Out of scope"
		o.Capability.ImplementationState = "out_of_scope"
		o.Capability.Command = ""
		o.Capability.Surface = "Out of scope"
		o.Capability.SafetyGuard = "Out of scope. TADX does not execute or hand off this operation."
	}
	return visibleOutput(o)
}

// FullOutput retains the complete bounded contract, including delegated
// capabilities, without applying the compact out-of-scope projection.
func (o Output) FullOutput() any { return visibleOutput(o) }
