package delete

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// Action deletes one authoritative Pulse metric.
type Action struct {
	reader  Reader
	deleter Deleter
}

// New creates a Pulse metric delete action.
func New(reader Reader, deleter Deleter) *Action { return &Action{reader: reader, deleter: deleter} }

// Execute reads and revalidates the exact target before deletion.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil || a.deleter == nil {
		return Output{}, failure("unconfigured", errs.KindRuntime, input, "Pulse metric delete is not configured.", nil)
	}
	input.LUID = strings.TrimSpace(input.LUID)
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return Output{}, failure("usage", errs.KindUsage, input, "Pulse metric delete requires an environment, site, and exact LUID.", nil)
	}
	target, err := a.reader.GetMetric(ctx, input.LUID)
	if err != nil {
		return Output{}, failure("resolve", errs.KindOperation, input, "Pulse metric resolution failed.", err)
	}
	if target.LUID != input.LUID {
		return Output{}, failure("identity", errs.KindOperation, input, "Tableau returned a different Pulse metric identity.", errors.New("exact target LUID mismatch"))
	}
	mode := "execute"
	if input.Preview {
		mode = "preview"
	}
	output := Output{
		Plan:     Plan{Mode: mode, Operation: "pulse.metric.delete", Environment: input.Environment, Site: input.Site, Target: target},
		Warnings: []string{"Tableau determines dependency and cascade effects. Dependent resources are not enumerated."},
		Help:     []string{commandhint.Environment(input.Environment, "pulse", "metric", "list", "--definition-id", target.DefinitionLUID)},
	}
	if target.IsDefault != nil && *target.IsDefault {
		output.Help = []string{"A default Pulse metric cannot be deleted independently; choose a non-default metric variant."}
		return output, defaultFailure(input, target)
	}
	if input.Preview {
		output.Help = []string{"Remove --preview to delete this exact Pulse metric."}
		return output, nil
	}
	if target.DefinitionLUID == "" {
		output.Help = nil
	}
	current, err := a.reader.GetMetric(ctx, input.LUID)
	if err != nil {
		return Output{}, failure("revalidate", errs.KindOperation, input, "Pulse metric revalidation failed.", err)
	}
	if current.LUID != target.LUID {
		return Output{}, failure("target_changed", errs.KindOperation, input, "The Pulse metric identity changed before deletion.", errors.New("exact target LUID changed"))
	}
	result, err := a.deleter.DeleteMetric(ctx, target.LUID)
	if err != nil {
		return Output{}, failure("failed", errs.KindOperation, input, "Pulse metric delete failed.", err)
	}
	output.Result = &result
	return output, nil
}

func defaultFailure(input Input, target Metric) error {
	return &errs.Error{
		ID:               "pulse.metric.delete.default",
		Kind:             errs.KindOperation,
		Operation:        "pulse.metric.delete",
		Resource:         target.LUID,
		Environment:      input.Environment,
		Site:             input.Site,
		Summary:          "The default Pulse metric cannot be deleted independently.",
		Cause:            fmt.Errorf("metric %q is the default variant for definition %q", target.LUID, target.DefinitionLUID),
		Retryable:        errs.Bool(false),
		CorrectiveAction: "Select a non-default Pulse metric variant, inspect it, and preview its exact deletion.",
		Phase:            errs.PhaseValidation,
		Outcome:          errs.OutcomeNotAttempted,
		TableauRequestID: "",
	}
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
		corrective = "Provide an environment, site, and exact Pulse metric LUID."
	}
	return &errs.Error{ID: "pulse.metric.delete." + suffix, Kind: kind, Operation: "pulse.metric.delete", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause), Phase: phase, Outcome: outcome}
}
