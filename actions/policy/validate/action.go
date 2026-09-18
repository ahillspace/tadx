// Package validate validates a candidate without activating or trusting it.
package validate

import (
	"context"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

type Input struct{ File string }
type Output struct {
	Status              string   `json:"status"`
	Candidate           string   `json:"candidate"`
	AllowedCapabilities int      `json:"allowed_capabilities"`
	RemoteMutations     bool     `json:"remote_mutations"`
	ProtectionChecked   bool     `json:"protection_checked"`
	Help                []string `json:"help"`
}
type Validator interface {
	ValidateCandidate(context.Context, string) (Output, error)
}
type Action struct{ validator Validator }

func New(validator Validator) *Action { return &Action{validator: validator} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if strings.TrimSpace(in.File) == "" {
		return Output{}, &errs.Error{ID: "policy.validate.usage", Kind: errs.KindUsage, Operation: "policy.validate", Summary: "An exact candidate file is required.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
	}
	if a == nil || a.validator == nil {
		return Output{}, &errs.Error{ID: "policy.validate.unconfigured", Kind: errs.KindRuntime, Operation: "policy.validate", Summary: "Policy validation is not configured.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return a.validator.ValidateCandidate(ctx, in.File)
}
