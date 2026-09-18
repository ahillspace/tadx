// Package create manages one exact permission rule.
package create

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const operation = "admin.permission.create"

type Input struct{ Environment, Site, ResourceKind, ResourceLUID, DefaultFor, PrincipalType, PrincipalLUID, PrincipalUsername, Capability, Mode string }
type Rule struct {
	ResourceKind  string `json:"resource_kind"`
	ResourceLUID  string `json:"resource_luid"`
	DefaultFor    string `json:"default_for,omitempty"`
	PrincipalType string `json:"principal_type"`
	PrincipalLUID string `json:"principal_luid"`
	Capability    string `json:"capability"`
	Mode          string `json:"mode"`
}
type Snapshot struct{ ResourceKind, ResourceLUID, Source, ParentProjectLUID, Mode string }
type Plan struct {
	Mode        string `json:"mode"`
	Operation   string `json:"operation"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	Target      Rule   `json:"target"`
	Source      string `json:"source"`
	CurrentMode string `json:"current_mode,omitempty"`
	Change      string `json:"change"`
}
type Result struct {
	Status           string `json:"status"`
	ResourceLUID     string `json:"resource_luid"`
	Rule             *Rule  `json:"rule,omitempty"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan
	Result *Result
	Help   []string
}
type CompactResult struct {
	Plan    Plan     `json:"plan"`
	Result  *Result  `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}
type FullResult struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *Result
	if o.Result != nil {
		copy := *o.Result
		copy.TableauRequestID = ""
		result = &copy
	}
	return CompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return FullResult{Plan: o.Plan, Result: o.Result, Help: o.Help} }

type Reader interface {
	GetPermission(context.Context, Input) (Snapshot, error)
}
type Writer interface {
	CreatePermission(context.Context, Input) (Result, error)
}
type Action struct {
	reader Reader
	writer Writer
}

func New(r Reader, w Writer) *Action { return &Action{reader: r, writer: w} }

func Validate(in Input) error {
	for _, v := range []struct{ field, value string }{{"environment", in.Environment}, {"id", in.ResourceLUID}, {"capability", in.Capability}} {
		if strings.TrimSpace(v.value) == "" || strings.TrimSpace(v.value) != v.value {
			err := failure(in, "usage", errs.KindUsage, "Permission mutation requires explicit, nonblank selectors.", "Provide --environment, --kind, --id, --principal-type, --principal-id, --capability, and --mode.")
			err.Validation = []errs.ValidationDetail{{Field: v.field, Code: "required", Message: "Provide an explicit value without surrounding whitespace."}}
			return err
		}
	}
	if (strings.TrimSpace(in.PrincipalLUID) == "") == (strings.TrimSpace(in.PrincipalUsername) == "") || (in.PrincipalUsername != "" && in.PrincipalType != "user") {
		return failure(in, "usage", errs.KindUsage, "Permission mutation requires an exact principal selector.", "Provide --principal-id, or --principal-type user with --principal-username.")
	}
	if (in.ResourceKind != "workbook" && in.ResourceKind != "datasource" && in.ResourceKind != "flow" && in.ResourceKind != "project") ||
		(in.PrincipalType != "user" && in.PrincipalType != "group") || (in.Mode != "Allow" && in.Mode != "Deny") {
		return failure(in, "usage", errs.KindUsage, "Invalid permission resource kind, principal type, or mode.", "Use workbook, datasource, flow, or project; user or group; and Allow or Deny.")
	}
	if in.DefaultFor != "" && (in.ResourceKind != "project" || (in.DefaultFor != "workbooks" && in.DefaultFor != "datasources" && in.DefaultFor != "flows")) {
		return failure(in, "usage", errs.KindUsage, "Invalid project default permission selector.", "Use --kind project with --default-for workbooks, datasources, or flows.")
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, errors.New("permission create is not configured")
	}
	initial, err := a.reader.GetPermission(ctx, in)
	if err != nil {
		return Output{}, err
	}
	if err := validateSnapshot(in, initial); err != nil {
		return Output{}, err
	}
	change := "create"
	if initial.Mode == in.Mode {
		change = "none"
	} else if initial.Mode != "" {
		return Output{}, failure(in, "rule_conflict", errs.KindOperation, "The capability already has a different explicit mode.", "Delete the existing capability and mode before creating the replacement.")
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: operation, Environment: in.Environment, Site: in.Site, Target: Rule{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor, PrincipalType: in.PrincipalType, PrincipalLUID: in.PrincipalLUID, Capability: in.Capability, Mode: in.Mode}, Source: initial.Source, CurrentMode: initial.Mode, Change: change}, Help: []string{permissionHint(in)}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.reader.GetPermission(ctx, in)
	if err != nil {
		return Output{}, err
	}
	if err := validateSnapshot(in, current); err != nil {
		return Output{}, err
	}
	if current != initial {
		return Output{}, failure(in, "target_changed", errs.KindOperation, "Permission rule changed during revalidation.", "Inspect the exact rule and run a new preview before retrying.")
	}
	if change == "none" {
		out.Result = &Result{Status: "unchanged", ResourceLUID: in.ResourceLUID, Rule: observedRule(in, initial.Mode)}
		return out, nil
	}
	result, err := a.writer.CreatePermission(ctx, in)
	if err != nil {
		if result.Status == "unknown" {
			e := failure(in, "outcome_unknown", errs.KindOperation, "The permission mutation outcome could not be determined safely.", "Inspect the exact rule and Tableau request before retrying.")
			e.Cause = err
			e.TableauRequestID = result.TableauRequestID
			return Output{}, e
		}
		return Output{}, err
	}
	if result.Status != "created" || result.ResourceLUID != in.ResourceLUID {
		e := failure(in, "outcome_unknown", errs.KindOperation, "Permission mutation returned an inconsistent result.", "Inspect the exact rule and Tableau request before retrying.")
		e.TableauRequestID = result.TableauRequestID
		return Output{}, e
	}
	out.Result = &result
	observed, err := a.reader.GetPermission(ctx, in)
	if err != nil {
		return out, confirmationFailure(in, result, err)
	}
	if err := validateSnapshot(in, observed); err != nil {
		return out, confirmationFailure(in, result, err)
	}
	result.Rule = observedRule(in, observed.Mode)
	if observed.Mode != in.Mode {
		return out, confirmationFailure(in, result, fmt.Errorf("post-write permission read returned mode %q, want %q", observed.Mode, in.Mode))
	}
	return out, nil
}

func observedRule(in Input, mode string) *Rule {
	if mode == "" {
		return nil
	}
	return &Rule{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor, PrincipalType: in.PrincipalType, PrincipalLUID: in.PrincipalLUID, Capability: in.Capability, Mode: mode}
}

func confirmationFailure(in Input, result Result, cause error) *errs.Error {
	return &errs.Error{ID: operation + ".verification_failed", Kind: errs.KindOperation, Operation: operation, Environment: in.Environment, Site: in.Site, Resource: in.ResourceLUID, Summary: "The permission write was acknowledged, but its exact saved rule could not be confirmed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact permission rule before retrying; do not repeat the acknowledged write automatically.", TableauRequestID: result.TableauRequestID, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed}
}
func validateSnapshot(in Input, s Snapshot) error {
	expectedSource := "direct"
	if in.DefaultFor != "" {
		expectedSource = "default"
	}
	if s.ResourceKind != in.ResourceKind || s.ResourceLUID != in.ResourceLUID {
		return failure(in, "identity_mismatch", errs.KindOperation, "Permission read returned a different resource identity.", "Inspect the exact resource before retrying.")
	}
	if s.Source != expectedSource || s.ParentProjectLUID != "" {
		return failure(in, "inherited_or_unknown", errs.KindOperation, "The permission rule is inherited or its source is unknown.", "Inspect the controlling project and target its explicit default permissions.")
	}
	if s.Mode != "" && s.Mode != "Allow" && s.Mode != "Deny" {
		return failure(in, "invalid_rule", errs.KindOperation, "Permission read returned an invalid rule mode.", "Inspect the upstream permission response before retrying.")
	}
	return nil
}
func failure(in Input, id string, kind errs.Kind, summary, advice string) *errs.Error {
	if kind == errs.KindOperation && in.ResourceLUID != "" {
		advice += " Run " + permissionHint(in) + "."
	}
	return &errs.Error{ID: operation + "." + id, Kind: kind, Operation: operation, Environment: in.Environment, Site: in.Site, Resource: in.ResourceLUID, Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: advice}
}
