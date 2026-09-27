package metric

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

// Delete reads and revalidates the exact target before deletion.
func Delete(ctx context.Context, reader DeleteReader, deleter Deleter, input DeleteInput) (DeleteOutput, error) {
	input.LUID = strings.TrimSpace(input.LUID)
	observed, err := reader.GetMetric(ctx, input.LUID)
	if err != nil {
		return DeleteOutput{}, deleteFailure("resolve", errs.KindOperation, input, "Pulse metric resolution failed.", err)
	}
	target := DeleteMetric{LUID: observed.LUID, Name: observed.Name, DefinitionLUID: observed.DefinitionLUID}
	if observed.DefaultKnown {
		target.IsDefault = new(observed.IsDefault)
	}
	if target.LUID != input.LUID {
		return DeleteOutput{}, deleteFailure("identity", errs.KindOperation, input, "Tableau returned a different Pulse metric identity.", errors.New("exact target LUID mismatch"))
	}
	mode := "execute"
	if input.Preview {
		mode = "preview"
	}
	output := DeleteOutput{
		Plan:     DeletePlan{Mode: mode, Operation: "pulse.metric.delete", Environment: input.Environment, Site: input.Site, Target: target},
		Warnings: []string{"Tableau determines dependency and cascade effects. Dependent resources are not enumerated."},
		Help:     []string{commandhint.Environment(input.Environment, "pulse", "metric", "list", "--definition-id", target.DefinitionLUID)},
	}
	if target.IsDefault != nil && *target.IsDefault {
		output.Help = []string{"A default Pulse metric cannot be deleted independently; choose a non-default metric variant."}
		return output, deleteDefaultFailure(input, target)
	}
	if input.Preview {
		output.Help = []string{"Remove --preview to delete this exact Pulse metric."}
		return output, nil
	}
	if target.DefinitionLUID == "" {
		output.Help = nil
	}
	current, err := reader.GetMetric(ctx, input.LUID)
	if err != nil {
		return DeleteOutput{}, deleteFailure("revalidate", errs.KindOperation, input, "Pulse metric revalidation failed.", err)
	}
	if current.LUID != target.LUID {
		return DeleteOutput{}, deleteFailure("target_changed", errs.KindOperation, input, "The Pulse metric identity changed before deletion.", errors.New("exact target LUID changed"))
	}
	result, err := deleter.DeleteMetric(ctx, target.LUID)
	if err != nil {
		return DeleteOutput{}, deleteFailure("failed", errs.KindOperation, input, "Pulse metric delete failed.", err)
	}
	output.Result = &result
	return output, nil
}

func deleteDefaultFailure(input DeleteInput, target DeleteMetric) error {
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

func deleteFailure(suffix string, kind errs.Kind, input DeleteInput, summary string, cause error) error {
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Inspect the remote delete outcome before retrying.")
	phase, outcome := errs.PhaseVerification, errs.OutcomeNotAttempted
	if suffix == "usage" {
		phase = errs.PhaseValidation
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

// Input identifies one authoritative target. Preview disables the mutation.
type DeleteInput struct {
	Environment string
	Site        string
	LUID        string
	Preview     bool
}

// Metric contains bounded authoritative target facts.
type DeleteMetric struct {
	LUID           string `json:"luid"`
	Name           string `json:"name,omitempty"`
	DefinitionLUID string `json:"definition_luid,omitempty"`
	IsDefault      *bool  `json:"is_default,omitempty"`
}

// Reader obtains one exact live target.
type DeleteReader interface {
	GetMetric(context.Context, string) (Metric, error)
}

// Deleter sends one exact delete request without cascading client-side.
type Deleter interface {
	DeleteMetric(context.Context, string) (DeleteResult, error)
}

// Plan describes the target selected for deletion.
type DeletePlan struct {
	Mode        string       `json:"mode"`
	Operation   string       `json:"operation"`
	Environment string       `json:"environment"`
	Site        string       `json:"site"`
	Target      DeleteMetric `json:"target"`
}

// Result preserves the upstream delete outcome.
type DeleteResult struct {
	Status           string `json:"status"`
	MetricLUID       string `json:"metric_luid"`
	HTTPStatus       int    `json:"http_status"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

// Output contains the exact plan and optional remote outcome.
type DeleteOutput struct {
	Plan     DeletePlan    `json:"plan"`
	Result   *DeleteResult `json:"result,omitempty"`
	Warnings []string      `json:"warnings"`
	Help     []string      `json:"help"`
}

// CompactDeleteResult retains the status and authoritative identity.
type DeleteCompactDeleteResult struct {
	Status     string `json:"status"`
	MetricLUID string `json:"metric_luid"`
}

// CompactResult omits bounded HTTP diagnostics.
type DeleteCompactResult struct {
	Plan     DeletePlan                 `json:"plan"`
	Result   *DeleteCompactDeleteResult `json:"result,omitempty"`
	Warnings []string                   `json:"warnings"`
	Details  string                     `json:"details"`
	Help     []string                   `json:"help"`
}

// CompactOutput projects bounded decision fields.
func (o DeleteOutput) CompactOutput() any {
	var result *DeleteCompactDeleteResult
	if o.Result != nil {
		result = &DeleteCompactDeleteResult{Status: o.Result.Status, MetricLUID: o.Result.MetricLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput includes the bounded upstream diagnostics for the same operation.
func (o DeleteOutput) FullOutput() any { return o }

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func DeleteValidateInput(input DeleteInput) error {
	if strings.TrimSpace(input.LUID) == "" {
		return deleteFailure("usage", errs.KindUsage, input, "Pulse metric delete requires an exact LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.LUID); err != nil {
		return deleteFailure("usage", errs.KindUsage, input, "Pulse metric delete requires a well-formed exact LUID.", err)
	}
	return nil
}
