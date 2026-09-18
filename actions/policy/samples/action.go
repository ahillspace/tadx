// Package samples creates deployable managed policy candidates without installing them.
package samples

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct{ OutputDirectory string }
type Output struct {
	Status       string   `json:"status"`
	Files        []string `json:"files"`
	SystemPath   string   `json:"system_path"`
	Instructions []string `json:"instructions"`
}
type Writer interface {
	WriteSamples(context.Context, string) (Output, error)
}
type Action struct{ writer Writer }

func New(writer Writer) *Action { return &Action{writer: writer} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if strings.TrimSpace(in.OutputDirectory) == "" {
		return Output{}, &errs.Error{ID: "policy.samples.usage", Kind: errs.KindUsage, Operation: "policy.samples", Summary: "--output is required.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
	}
	if a == nil || a.writer == nil {
		return Output{}, &errs.Error{ID: "policy.samples.unconfigured", Kind: errs.KindRuntime, Operation: "policy.samples", Summary: "Policy sample storage is not configured.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return a.writer.WriteSamples(ctx, in.OutputDirectory)
}
