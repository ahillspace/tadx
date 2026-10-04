package admin

import (
	"context"

	permission "github.com/ahillspace/tadx/actions/admin/permission"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

// PermissionPorts translates native permission records for action-owned workflows.
type PermissionPorts struct{ Adapter *Adapter }

func (p PermissionPorts) ResolvePrincipalUsername(ctx context.Context, username string) (string, error) {
	user, err := p.Adapter.ResolveUser(ctx, UserSelector{Username: username})
	return user.LUID, err
}

func (p PermissionPorts) GetPermissions(ctx context.Context, in permission.InspectInput) (permission.InspectPermissionSet, error) {
	set, err := p.Adapter.GetPermissions(ctx, tableau.PermissionRequest{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor})
	rules := make([]permission.InspectRule, len(set.Rules))
	for i, rule := range set.Rules {
		rules[i] = permission.InspectRule{PrincipalType: rule.PrincipalType, PrincipalLUID: rule.PrincipalLUID, Capability: rule.Capability, Mode: rule.Mode}
	}
	return permission.InspectPermissionSet{ResourceKind: set.ResourceKind, ResourceLUID: set.ResourceLUID, Source: set.Source, ParentProjectLUID: set.ParentProjectLUID, Rules: rules, RequestID: set.RequestID}, err
}

func (p PermissionPorts) GetPermission(ctx context.Context, in permission.Input) (permission.Snapshot, error) {
	set, err := p.Adapter.GetPermissionRule(ctx, permissionRequest(in))
	if err != nil {
		return permission.Snapshot{}, err
	}
	return permission.Snapshot{ResourceKind: set.ResourceKind, ResourceLUID: set.ResourceLUID, Source: set.Source, ParentProjectLUID: set.ParentProjectLUID, Mode: permissionRuleMode(set, in.PrincipalType, in.PrincipalLUID, in.Capability)}, nil
}

func (p PermissionPorts) CreatePermission(ctx context.Context, in permission.Input) (permission.Result, error) {
	result, err := p.Adapter.CreatePermission(ctx, permissionRequest(in))
	return permission.Result{Status: result.Status, ResourceLUID: result.ResourceLUID, TableauRequestID: result.RequestID}, err
}

func (p PermissionPorts) DeletePermission(ctx context.Context, in permission.Input) (permission.Result, error) {
	result, err := p.Adapter.DeletePermission(ctx, permissionRequest(in))
	return permission.Result{Status: result.Status, ResourceLUID: result.ResourceLUID, TableauRequestID: result.RequestID}, err
}

func permissionRequest(in permission.Input) tableau.PermissionMutationRequest {
	return tableau.PermissionMutationRequest{PermissionRequest: tableau.PermissionRequest{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor}, Rule: tableau.PermissionRule{PrincipalType: in.PrincipalType, PrincipalLUID: in.PrincipalLUID, Capability: in.Capability, Mode: in.Mode}}
}

func permissionRuleMode(set tableau.PermissionSet, principalType, principalLUID, capability string) string {
	for _, rule := range set.Rules {
		if rule.PrincipalType == principalType && rule.PrincipalLUID == principalLUID && rule.Capability == capability {
			return rule.Mode
		}
	}
	return ""
}
