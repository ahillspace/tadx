package permission

import (
	"context"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
)

const deleteOperation = "admin.permission.delete"

type DeleteWriter interface {
	DeletePermission(context.Context, Input) (Result, error)
}
type DeleteAction struct {
	reader Reader
	writer DeleteWriter
}

func NewDelete(r Reader, w DeleteWriter) *DeleteAction { return &DeleteAction{reader: r, writer: w} }

func (a *DeleteAction) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if err := ValidateDeleteInput(in); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, errors.New("permission delete is not configured")
	}
	initial, err := a.reader.GetPermission(ctx, in)
	if err != nil {
		return Output{}, err
	}
	if err := validateSnapshot(deleteOperation, in, initial); err != nil {
		return Output{}, err
	}
	change := "delete"
	if initial.Mode == "" {
		change = "none"
	} else if initial.Mode != in.Mode {
		return Output{}, failure(deleteOperation, in, "mode_mismatch", errs.KindOperation, "The existing capability mode differs from the requested deletion.", "Inspect the exact rule and supply its current Allow or Deny mode.")
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: deleteOperation, Environment: in.Environment, Site: in.Site, Target: Rule{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor, PrincipalType: in.PrincipalType, PrincipalLUID: in.PrincipalLUID, Capability: in.Capability, Mode: in.Mode}, Source: initial.Source, CurrentMode: initial.Mode, Change: change}, Help: []string{permissionHint(in)}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.reader.GetPermission(ctx, in)
	if err != nil {
		return Output{}, err
	}
	if err := validateSnapshot(deleteOperation, in, current); err != nil {
		return Output{}, err
	}
	if current != initial {
		return Output{}, failure(deleteOperation, in, "target_changed", errs.KindOperation, "Permission rule changed during revalidation.", "Inspect the exact rule and run a new preview before retrying.")
	}
	if change == "none" {
		out.Result = &Result{Status: "unchanged", ResourceLUID: in.ResourceLUID}
		return out, nil
	}
	result, err := a.writer.DeletePermission(ctx, in)
	if err != nil {
		if result.Status == "unknown" {
			e := failure(deleteOperation, in, "outcome_unknown", errs.KindOperation, "The permission mutation outcome could not be determined safely.", "Inspect the exact rule and Tableau request before retrying.")
			e.Cause = err
			e.TableauRequestID = result.TableauRequestID
			return Output{}, e
		}
		return Output{}, err
	}
	if result.Status != "deleted" || result.ResourceLUID != in.ResourceLUID {
		e := failure(deleteOperation, in, "outcome_unknown", errs.KindOperation, "Permission mutation returned an inconsistent result.", "Inspect the exact rule and Tableau request before retrying.")
		e.TableauRequestID = result.TableauRequestID
		return Output{}, e
	}
	out.Result = &result
	observed, err := a.reader.GetPermission(ctx, in)
	if err != nil {
		return out, deleteConfirmationFailure(in, result, err)
	}
	if err := validateSnapshot(deleteOperation, in, observed); err != nil {
		return out, deleteConfirmationFailure(in, result, err)
	}
	result.Rule = observedRule(in, observed.Mode)
	if observed.Mode != "" {
		return out, deleteConfirmationFailure(in, result, fmt.Errorf("post-write permission read still returned mode %q", observed.Mode))
	}
	return out, nil
}

func deleteConfirmationFailure(in Input, result Result, cause error) *errs.Error {
	return &errs.Error{ID: deleteOperation + ".verification_failed", Kind: errs.KindOperation, Operation: deleteOperation, Environment: in.Environment, Site: in.Site, Resource: in.ResourceLUID, Summary: "The permission delete was acknowledged, but its exact absence could not be confirmed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact permission rule before retrying; do not repeat the acknowledged delete automatically.", TableauRequestID: result.TableauRequestID, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed}
}

func ValidateDeleteInput(in Input) error { return validateInput(deleteOperation, in) }
