package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"sort"
	"strings"
)

type Input struct{ Environment, Site, ResourceKind, ResourceLUID, DefaultFor, PrincipalType, PrincipalLUID, PrincipalUsername, Capability string }
type Rule struct {
	Source        string `json:"source"`
	PrincipalType string `json:"principal_type"`
	PrincipalLUID string `json:"principal_luid"`
	Capability    string `json:"capability"`
	Mode          string `json:"mode"`
}
type PermissionSet struct {
	ResourceKind      string `json:"resource_kind"`
	ResourceLUID      string `json:"resource_luid"`
	Source            string `json:"source"`
	ParentProjectLUID string `json:"parent_project_luid,omitempty"`
	Rules             []Rule `json:"rules"`
	RulesOmitted      int    `json:"rules_omitted,omitempty"`
	RequestID         string `json:"-"`
}
type Output struct {
	Status, Environment, Site string
	Permissions               PermissionSet
	RequestID                 string
	Help                      []string
}
type CompactPermissions struct {
	ResourceKind string `json:"resource_kind"`
	ResourceLUID string `json:"resource_luid"`
	Source       string `json:"source"`
	RuleCount    int    `json:"rule_count"`
}
type CompactResult struct {
	Status      string             `json:"status"`
	Environment string             `json:"environment,omitempty"`
	Site        string             `json:"site,omitempty"`
	Permissions CompactPermissions `json:"permissions"`
	Details     string             `json:"details"`
	Help        []string           `json:"help"`
}
type FullResult struct {
	Status      string        `json:"status"`
	Environment string        `json:"environment,omitempty"`
	Site        string        `json:"site,omitempty"`
	Permissions PermissionSet `json:"permissions"`
	RequestID   string        `json:"tableau_request_id,omitempty"`
	Help        []string      `json:"help"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Permissions: CompactPermissions{ResourceKind: o.Permissions.ResourceKind, ResourceLUID: o.Permissions.ResourceLUID, Source: o.Permissions.Source, RuleCount: len(o.Permissions.Rules)}, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	p := o.Permissions
	p.Rules = append([]Rule(nil), p.Rules...)
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Permissions: p, RequestID: o.RequestID, Help: o.Help}
}

type Reader interface {
	GetPermissions(context.Context, Input) (PermissionSet, error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, errors.New("admin permission reader is not configured")
	}
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	p, err := a.reader.GetPermissions(ctx, in)
	if err != nil {
		return Output{}, err
	}
	filtered := make([]Rule, 0, len(p.Rules))
	for _, rule := range p.Rules {
		if in.PrincipalType != "" && rule.PrincipalType != in.PrincipalType {
			continue
		}
		if in.PrincipalLUID != "" && rule.PrincipalLUID != in.PrincipalLUID {
			continue
		}
		if in.Capability != "" && rule.Capability != in.Capability {
			continue
		}
		rule.Source = p.Source
		filtered = append(filtered, rule)
	}
	p.Rules = filtered
	sort.Slice(p.Rules, func(i, j int) bool {
		a, b := p.Rules[i], p.Rules[j]
		if a.PrincipalType != b.PrincipalType {
			return a.PrincipalType < b.PrincipalType
		}
		if a.PrincipalLUID != b.PrincipalLUID {
			return a.PrincipalLUID < b.PrincipalLUID
		}
		if a.Capability != b.Capability {
			return a.Capability < b.Capability
		}
		return a.Mode < b.Mode
	})
	return Output{Status: "found", Environment: in.Environment, Site: in.Site, Permissions: p, RequestID: p.RequestID, Help: []string{permissionHint(in)}}, nil
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

// ValidateInput checks local permission filters without a remote session.
func ValidateInput(in Input) error {
	validKind := in.ResourceKind == "workbook" || in.ResourceKind == "datasource" || in.ResourceKind == "flow" || in.ResourceKind == "project"
	validDefault := in.DefaultFor == "" || in.ResourceKind == "project" && (in.DefaultFor == "workbooks" || in.DefaultFor == "datasources" || in.DefaultFor == "flows")
	validPrincipal := in.PrincipalType == "" || in.PrincipalType == "user" || in.PrincipalType == "group"
	validUsername := in.PrincipalUsername == "" || (in.PrincipalType == "user" && in.PrincipalLUID == "")
	if !validKind || strings.TrimSpace(in.ResourceLUID) == "" || !validDefault || !validPrincipal || !validUsername {
		return &errs.Error{ID: "admin.permission.inspect.usage", Kind: errs.KindUsage, Operation: "admin.permission.inspect", Summary: "Invalid permission inspection selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Use --kind workbook, datasource, flow, or project with an exact --id; --default-for applies only to projects and --principal-type is user or group."}
	}
	return nil
}
