package get

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	return Output{Capability: item, Help: []string{"tadx capability list"}}, nil
}
