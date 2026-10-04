package permission

import (
	"context"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type InspectInput struct{ Environment, Site, ResourceKind, ResourceLUID, DefaultFor, PrincipalType, PrincipalLUID, PrincipalUsername, Capability string }
type InspectRule struct {
	Source        string `json:"source"`
	PrincipalType string `json:"principal_type"`
	PrincipalLUID string `json:"principal_luid"`
	Capability    string `json:"capability"`
	Mode          string `json:"mode"`
}
type InspectPermissionSet struct {
	ResourceKind      string        `json:"resource_kind"`
	ResourceLUID      string        `json:"resource_luid"`
	Source            string        `json:"source"`
	ParentProjectLUID string        `json:"parent_project_luid,omitempty"`
	Rules             []InspectRule `json:"rules"`
	RulesOmitted      int           `json:"rules_omitted,omitempty"`
	RequestID         string        `json:"-"`
}
type InspectOutput struct {
	Status, Environment, Site string
	Permissions               InspectPermissionSet
	RequestID                 string
	Help                      []string
}
type InspectCompactPermissions struct {
	ResourceKind string `json:"resource_kind"`
	ResourceLUID string `json:"resource_luid"`
	Source       string `json:"source"`
	RuleCount    int    `json:"rule_count"`
}
type InspectCompactResult struct {
	Status      string                    `json:"status"`
	Environment string                    `json:"environment,omitempty"`
	Site        string                    `json:"site,omitempty"`
	Permissions InspectCompactPermissions `json:"permissions"`
	Details     string                    `json:"details"`
	Help        []string                  `json:"help"`
}
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Permissions InspectPermissionSet `json:"permissions"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
}

func (o InspectOutput) CompactOutput() any {
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Permissions: InspectCompactPermissions{ResourceKind: o.Permissions.ResourceKind, ResourceLUID: o.Permissions.ResourceLUID, Source: o.Permissions.Source, RuleCount: len(o.Permissions.Rules)}, Details: "--full", Help: o.Help}
}
func (o InspectOutput) FullOutput() any {
	p := o.Permissions
	p.Rules = append([]InspectRule(nil), p.Rules...)
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Permissions: p, RequestID: o.RequestID, Help: o.Help}
}

type InspectReader interface {
	GetPermissions(context.Context, InspectInput) (InspectPermissionSet, error)
}

func InspectPermissions(ctx context.Context, reader InspectReader, in InspectInput) (InspectOutput, error) {
	p, err := reader.GetPermissions(ctx, in)
	if err != nil {
		return InspectOutput{}, err
	}
	filtered := make([]InspectRule, 0, len(p.Rules))
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
	return InspectOutput{Status: "found", Environment: in.Environment, Site: in.Site, Permissions: p, RequestID: p.RequestID, Help: []string{inspectPermissionHint(in)}}, nil
}

func inspectPermissionHint(in InspectInput) string {
	args := []string{"admin", "permission", "inspect", "--kind", in.ResourceKind, "--id", in.ResourceLUID, "--full"}
	for _, flag := range []struct{ name, value string }{{"--default-for", in.DefaultFor}, {"--principal-type", in.PrincipalType}, {"--principal-id", in.PrincipalLUID}, {"--capability", in.Capability}} {
		if flag.value != "" {
			args = append(args, flag.name, flag.value)
		}
	}
	return commandhint.Environment(in.Environment, args...)
}

// ValidateInspectInput checks local permission filters without a remote session.
func ValidateInspectInput(in InspectInput) error {
	validKind := in.ResourceKind == "workbook" || in.ResourceKind == "datasource" || in.ResourceKind == "flow" || in.ResourceKind == "project"
	validDefault := in.DefaultFor == "" || in.ResourceKind == "project" && (in.DefaultFor == "workbooks" || in.DefaultFor == "datasources" || in.DefaultFor == "flows")
	validPrincipal := in.PrincipalType == "" || in.PrincipalType == "user" || in.PrincipalType == "group"
	validUsername := in.PrincipalUsername == "" || (in.PrincipalType == "user" && in.PrincipalLUID == "")
	if !validKind || strings.TrimSpace(in.ResourceLUID) == "" || !validDefault || !validPrincipal || !validUsername {
		return &errs.Error{ID: "admin.permission.inspect.usage", Kind: errs.KindUsage, Operation: "admin.permission.inspect", Summary: "Invalid permission inspection selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Use --kind workbook, datasource, flow, or project with an exact --id; --default-for applies only to projects and --principal-type is user or group."}
	}
	return nil
}
