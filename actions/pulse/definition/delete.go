package definition

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// Delete reads and revalidates the exact target before deletion.
func runDelete(ctx context.Context, reader DeleteReader, deleter Deleter, input DeleteInput) (DeleteOutput, error) {
	input.LUID = strings.TrimSpace(input.LUID)
	target, err := reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		return DeleteOutput{}, deleteFailure("resolve", errs.KindOperation, input, "Pulse definition resolution failed.", err)
	}
	if target.LUID != input.LUID {
		return DeleteOutput{}, deleteFailure("identity", errs.KindOperation, input, "Tableau returned a different Pulse definition identity.", errors.New("exact target LUID mismatch"))
	}
	mode := "execute"
	if input.Preview {
		mode = "preview"
	}
	output := DeleteOutput{
		Plan:     DeletePlan{Mode: mode, Operation: "pulse.definition.delete", Environment: input.Environment, Site: input.Site, Target: DeleteDefinition{LUID: target.LUID, Name: target.Name, DatasourceLUID: target.DatasourceLUID}},
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
	current, err := reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		return DeleteOutput{}, deleteFailure("revalidate", errs.KindOperation, input, "Pulse definition revalidation failed.", err)
	}
	if current.LUID != target.LUID {
		return DeleteOutput{}, deleteFailure("target_changed", errs.KindOperation, input, "The Pulse definition identity changed before deletion.", errors.New("exact target LUID changed"))
	}
	result, err := deleter.DeleteDefinition(ctx, target.LUID)
	if err != nil {
		return DeleteOutput{}, deleteFailure("failed", errs.KindOperation, input, "Pulse definition delete failed.", err)
	}
	output.Result = &result
	return output, nil
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
		corrective = "Provide an environment, site, and exact Pulse definition LUID."
	}
	return &errs.Error{ID: "pulse.definition.delete." + suffix, Kind: kind, Operation: "pulse.definition.delete", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause), Phase: phase, Outcome: outcome}
}

// Input identifies one authoritative target. Preview disables the mutation.
type DeleteInput struct {
	Environment string
	Site        string
	LUID        string
	Preview     bool
}

// Definition contains bounded authoritative target facts.
type DeleteDefinition struct {
	LUID           string `json:"luid"`
	Name           string `json:"name,omitempty"`
	DatasourceLUID string `json:"datasource_luid,omitempty"`
}

// Reader obtains one exact live target.
type DeleteReader interface {
	GetDefinition(context.Context, string) (Definition, error)
}

// Deleter sends one exact delete request without cascading client-side.
type Deleter interface {
	DeleteDefinition(context.Context, string) (DeleteResult, error)
}

// Plan describes the target selected for deletion.
type DeletePlan struct {
	Mode        string           `json:"mode"`
	Operation   string           `json:"operation"`
	Environment string           `json:"environment"`
	Site        string           `json:"site"`
	Target      DeleteDefinition `json:"target"`
}

// Result preserves the upstream delete outcome.
type DeleteResult struct {
	Status           string `json:"status"`
	DefinitionLUID   string `json:"definition_luid"`
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
	Status         string `json:"status"`
	DefinitionLUID string `json:"definition_luid"`
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
		result = &DeleteCompactDeleteResult{Status: o.Result.Status, DefinitionLUID: o.Result.DefinitionLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput includes the bounded upstream diagnostics for the same operation.
func (o DeleteOutput) FullOutput() any { return o }

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func deleteValidateInput(input DeleteInput) error {
	if strings.TrimSpace(input.LUID) == "" {
		return deleteFailure("usage", errs.KindUsage, input, "Pulse definition delete requires an exact LUID.", nil)
	}
	return nil
}
