package delete

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// Action deletes one authoritative Pulse definition.
type Action struct {
	reader  Reader
	deleter Deleter
}

// New creates a Pulse definition delete action.
func New(reader Reader, deleter Deleter) *Action { return &Action{reader: reader, deleter: deleter} }

// Execute reads and revalidates the exact target before deletion.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil || a.deleter == nil {
		return Output{}, failure("unconfigured", errs.KindRuntime, input, "Pulse definition delete is not configured.", nil)
	}
	input.LUID = strings.TrimSpace(input.LUID)
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) || input.LUID == "" {
		return Output{}, failure("usage", errs.KindUsage, input, "Pulse definition delete requires an environment, site, and exact LUID.", nil)
	}
	target, err := a.reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		return Output{}, failure("resolve", errs.KindOperation, input, "Pulse definition resolution failed.", err)
	}
	if target.LUID != input.LUID {
		return Output{}, failure("identity", errs.KindOperation, input, "Tableau returned a different Pulse definition identity.", errors.New("exact target LUID mismatch"))
	}
	mode := "execute"
	if input.Preview {
		mode = "preview"
	}
	output := Output{
		Plan:     Plan{Mode: mode, Operation: "pulse.definition.delete", Environment: input.Environment, Site: input.Site, Target: target},
		Warnings: []string{"Deletes this definition and Tableau-managed dependents. Tableau determines the complete cascade; dependent resources are not enumerated."},
		Help:     []string{commandhint.Environment(input.Environment, "pulse", "definition", "list", "--datasource-id", target.DatasourceLUID)},
	}
	if input.Preview {
		output.Help = []string{"Remove --preview to delete this exact Pulse definition."}
		return output, nil
	}
	if target.DatasourceLUID == "" {
		output.Help = nil
	}
	current, err := a.reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		return Output{}, failure("revalidate", errs.KindOperation, input, "Pulse definition revalidation failed.", err)
	}
	if current.LUID != target.LUID {
		return Output{}, failure("target_changed", errs.KindOperation, input, "The Pulse definition identity changed before deletion.", errors.New("exact target LUID changed"))
	}
	result, err := a.deleter.DeleteDefinition(ctx, target.LUID)
	if err != nil {
		return Output{}, failure("failed", errs.KindOperation, input, "Pulse definition delete failed.", err)
	}
	output.Result = &result
	return output, nil
}

func failure(suffix string, kind errs.Kind, input Input, summary string, cause error) error {
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Inspect the remote delete outcome before retrying.")
	phase, outcome := errs.PhaseVerification, errs.OutcomeNotAttempted
	if suffix == "usage" {
		phase = errs.PhaseValidation
	}
	if suffix == "unconfigured" {
		phase = errs.PhaseSetup
	}
	if suffix == "failed" {
		phase, outcome = errs.PhaseSubmission, errs.OutcomeUnknown
	}
	if kind == errs.KindUsage {
		retryable = errs.Bool(false)
		corrective = "Provide an environment, site, and exact Pulse definition LUID."
	}
	return &errs.Error{ID: "pulse.definition.delete." + suffix, Kind: kind, Operation: "pulse.definition.delete", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause), Phase: phase, Outcome: outcome}
}
