package permission

import (
	"context"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

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

func validateInput(operation string, in Input) error {
	for _, v := range []struct{ field, value string }{{"environment", in.Environment}, {"id", in.ResourceLUID}, {"capability", in.Capability}} {
		if strings.TrimSpace(v.value) == "" || strings.TrimSpace(v.value) != v.value {
			err := failure(operation, in, "usage", errs.KindUsage, "Permission mutation requires explicit, nonblank selectors.", "Provide --environment, --kind, --id, --principal-type, --principal-id, --capability, and --mode.")
			err.Validation = []errs.ValidationDetail{{Field: v.field, Code: "required", Message: "Provide an explicit value without surrounding whitespace."}}
			return err
		}
	}
	if (strings.TrimSpace(in.PrincipalLUID) == "") == (strings.TrimSpace(in.PrincipalUsername) == "") || (in.PrincipalUsername != "" && in.PrincipalType != "user") {
		return failure(operation, in, "usage", errs.KindUsage, "Permission mutation requires an exact principal selector.", "Provide --principal-id, or --principal-type user with --principal-username.")
	}
	if (in.ResourceKind != "workbook" && in.ResourceKind != "datasource" && in.ResourceKind != "flow" && in.ResourceKind != "project") ||
		(in.PrincipalType != "user" && in.PrincipalType != "group") || (in.Mode != "Allow" && in.Mode != "Deny") {
		return failure(operation, in, "usage", errs.KindUsage, "Invalid permission resource kind, principal type, or mode.", "Use workbook, datasource, flow, or project; user or group; and Allow or Deny.")
	}
	if in.DefaultFor != "" && (in.ResourceKind != "project" || (in.DefaultFor != "workbooks" && in.DefaultFor != "datasources" && in.DefaultFor != "flows")) {
		return failure(operation, in, "usage", errs.KindUsage, "Invalid project default permission selector.", "Use --kind project with --default-for workbooks, datasources, or flows.")
	}
	return nil
}
func observedRule(in Input, mode string) *Rule {
	if mode == "" {
		return nil
	}
	return &Rule{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor, PrincipalType: in.PrincipalType, PrincipalLUID: in.PrincipalLUID, Capability: in.Capability, Mode: mode}
}

func validateSnapshot(operation string, in Input, s Snapshot) error {
	expectedSource := "direct"
	if in.DefaultFor != "" {
		expectedSource = "default"
	}
	if s.ResourceKind != in.ResourceKind || s.ResourceLUID != in.ResourceLUID {
		return failure(operation, in, "identity_mismatch", errs.KindOperation, "Permission read returned a different resource identity.", "Inspect the exact resource before retrying.")
	}
	if s.Source != expectedSource || s.ParentProjectLUID != "" {
		return failure(operation, in, "inherited_or_unknown", errs.KindOperation, "The permission rule is inherited or its source is unknown.", "Inspect the controlling project and target its explicit default permissions.")
	}
	if s.Mode != "" && s.Mode != "Allow" && s.Mode != "Deny" {
		return failure(operation, in, "invalid_rule", errs.KindOperation, "Permission read returned an invalid rule mode.", "Inspect the upstream permission response before retrying.")
	}
	return nil
}
func failure(operation string, in Input, id string, kind errs.Kind, summary, advice string) *errs.Error {
	if kind == errs.KindOperation && in.ResourceLUID != "" {
		advice += " Run " + permissionHint(in) + "."
	}
	return &errs.Error{ID: operation + "." + id, Kind: kind, Operation: operation, Environment: in.Environment, Site: in.Site, Resource: in.ResourceLUID, Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: advice}
}
func permissionHint(in Input) string {
	args := []string{"admin", "permission", "inspect", "--kind", in.ResourceKind, "--id", in.ResourceLUID, "--full"}
	for _, flag := range []struct{ name, value string }{{"--default-for", in.DefaultFor}, {"--principal-type", in.PrincipalType}, {"--principal-id", in.PrincipalLUID}, {"--capability", in.Capability}} {
		if flag.value != "" {
			args = append(args, flag.name, flag.value)
		}
	}
	return commandhint.Environment(in.Environment, args...)
}
