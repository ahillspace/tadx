package permission

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
)

const createOperation = "admin.permission.create"

type CreateWriter interface {
	CreatePermission(context.Context, Input) (Result, error)
}

func Create(ctx context.Context, reader Reader, writer CreateWriter, in Input, preview bool) (Output, error) {
	initial, err := reader.GetPermission(ctx, in)
	if err != nil {
		return Output{}, err
	}
	if err := validateSnapshot(createOperation, in, initial); err != nil {
		return Output{}, err
	}
	change := "create"
	if initial.Mode == in.Mode {
		change = "none"
	} else if initial.Mode != "" {
		return Output{}, failure(createOperation, in, "rule_conflict", errs.KindOperation, "The capability already has a different explicit mode.", "Delete the existing capability and mode before creating the replacement.")
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: createOperation, Environment: in.Environment, Site: in.Site, Target: Rule{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor, PrincipalType: in.PrincipalType, PrincipalLUID: in.PrincipalLUID, Capability: in.Capability, Mode: in.Mode}, Source: initial.Source, CurrentMode: initial.Mode, Change: change}, Help: []string{permissionHint(in)}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := reader.GetPermission(ctx, in)
	if err != nil {
		return Output{}, err
	}
	if err := validateSnapshot(createOperation, in, current); err != nil {
		return Output{}, err
	}
	if current != initial {
		return Output{}, failure(createOperation, in, "target_changed", errs.KindOperation, "Permission rule changed during revalidation.", "Inspect the exact rule and run a new preview before retrying.")
	}
	if change == "none" {
		out.Result = &Result{Status: "unchanged", ResourceLUID: in.ResourceLUID, Rule: observedRule(in, initial.Mode)}
		return out, nil
	}
	result, err := writer.CreatePermission(ctx, in)
	if err != nil {
		if result.Status == "unknown" {
			e := failure(createOperation, in, "outcome_unknown", errs.KindOperation, "The permission mutation outcome could not be determined safely.", "Inspect the exact rule and Tableau request before retrying.")
			e.Cause = err
			e.TableauRequestID = result.TableauRequestID
			return Output{}, e
		}
		return Output{}, err
	}
	out.Result = &result
	observed, err := reader.GetPermission(ctx, in)
	if err != nil {
		return out, createConfirmationFailure(in, result, err)
	}
	if err := validateSnapshot(createOperation, in, observed); err != nil {
		return out, createConfirmationFailure(in, result, err)
	}
	result.Rule = observedRule(in, observed.Mode)
	if observed.Mode != in.Mode {
		return out, createConfirmationFailure(in, result, fmt.Errorf("post-write permission read returned mode %q, want %q", observed.Mode, in.Mode))
	}
	return out, nil
}

func createConfirmationFailure(in Input, result Result, cause error) *errs.Error {
	return &errs.Error{ID: createOperation + ".verification_failed", Kind: errs.KindOperation, Operation: createOperation, Environment: in.Environment, Site: in.Site, Resource: in.ResourceLUID, Summary: "The permission write was acknowledged, but its exact saved rule could not be confirmed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact permission rule before retrying; do not repeat the acknowledged write automatically.", TableauRequestID: result.TableauRequestID, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed}
}

func ValidateCreateInput(in Input) error { return validateInput(createOperation, in) }
