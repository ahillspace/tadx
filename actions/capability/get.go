package capability

import (
	"errors"
	"fmt"
	"strings"

	registry "github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/errs"
)

var (
	// ErrIDRequired identifies a missing registry ID.
	ErrIDRequired = errors.New("capability ID is required")
	// ErrNotFound identifies an unknown exact registry ID.
	ErrNotFound = errors.New("capability not found")
)

// getFromItem projects one exact capability without a second registry lookup.
func getFromItem(input GetInput, item Capability, ok, mutationsEnabled bool) (GetOutput, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return GetOutput{}, &errs.Error{
			ID:         "capability.get.usage",
			Kind:       errs.KindUsage,
			Operation:  "capability.get",
			Summary:    ErrIDRequired.Error(),
			Cause:      ErrIDRequired,
			Validation: []errs.ValidationDetail{{Field: "id", Code: "required", Message: ErrIDRequired.Error()}},
		}
	}
	if !ok {
		return GetOutput{}, &errs.Error{
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
	item.ExecutionEnabled = !item.PolicyDenied && item.ImplementationState == "implemented" && (!item.RemoteMutation || mutationsEnabled)
	return GetOutput{Capability: item, Help: []string{"tadx capability list"}}, nil
}

// Input selects one capability by its exact registry ID.
type GetInput struct {
	ID          string `json:"id"`
	Environment string `json:"-"`
}

// Capability is the detailed discovery view of one registry entry.
//
// The neutral representation is shared with capability.list full output so
// that the two commands cannot drift in their contract fields.
type Capability = registry.Discovery

// Output is the stable capability detail result.
type GetOutput struct {
	Capability Capability `json:"capability"`
	Help       []string   `json:"help"`
}

func (o GetOutput) CompactOutput() any {
	if o.Capability.Disposition == "delegated" {
		o.Capability.Disposition = "Out of scope"
		o.Capability.ImplementationState = "out_of_scope"
		o.Capability.Command = ""
		o.Capability.Surface = "Out of scope"
		o.Capability.SafetyGuard = "Out of scope. TADX does not execute or hand off this operation."
	}
	return o
}

// FullOutput retains the complete bounded contract, including delegated
// capabilities, without applying the compact out-of-scope projection.
func (o GetOutput) FullOutput() any { return o }
