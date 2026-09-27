package app

import (
	"context"
	"errors"

	permission "github.com/ahillspace/tadx/actions/admin/permission"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
)

func (c *remoteAdminCommands) CreateAdminPermission(ctx context.Context, in permission.Input, preview bool) (permission.Output, error) {
	connection, err := c.connect(ctx, in.Environment, true)
	if err != nil {
		return permission.Output{}, remoteSetupError("admin.permission.create", in.Environment, in.Site, connection.environment, err)
	}
	in.Environment, in.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if in.PrincipalUsername != "" {
		user, resolveErr := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: in.PrincipalUsername})
		if resolveErr != nil {
			return permission.Output{}, adminActionError("admin.permission.create", in.Environment, in.Site, resolveErr)
		}
		in.PrincipalLUID, in.PrincipalUsername = user.LUID, ""
	}
	adapter := adminPermissionMutationAdapter{connection.adapter}
	out, err := permission.Create(ctx, adapter, adapter, in, preview)
	return out, permissionMutationError("admin.permission.create", in.Environment, in.Site, err)
}

type adminPermissionMutationAdapter struct{ adapter *resourceadmin.Adapter }

func (a adminPermissionMutationAdapter) GetPermission(ctx context.Context, in permission.Input) (permission.Snapshot, error) {
	set, err := a.adapter.GetPermissionRule(ctx, permissionMutationRequest(in))
	if err != nil {
		return permission.Snapshot{}, err
	}
	mode := permissionRuleMode(set, in.PrincipalType, in.PrincipalLUID, in.Capability)
	return permission.Snapshot{ResourceKind: set.ResourceKind, ResourceLUID: set.ResourceLUID, Source: set.Source, ParentProjectLUID: set.ParentProjectLUID, Mode: mode}, nil
}
func (a adminPermissionMutationAdapter) CreatePermission(ctx context.Context, in permission.Input) (permission.Result, error) {
	result, err := a.adapter.CreatePermission(ctx, permissionMutationRequest(in))
	return permission.Result{Status: result.Status, ResourceLUID: result.ResourceLUID, TableauRequestID: result.RequestID}, err
}
func permissionMutationRequest(in permission.Input) tableauadmin.PermissionMutationRequest {
	return tableauadmin.PermissionMutationRequest{PermissionRequest: tableauadmin.PermissionRequest{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, DefaultFor: in.DefaultFor}, Rule: tableauadmin.PermissionRule{PrincipalType: in.PrincipalType, PrincipalLUID: in.PrincipalLUID, Capability: in.Capability, Mode: in.Mode}}
}

func (c *remoteAdminCommands) DeleteAdminPermission(ctx context.Context, in permission.Input, preview bool) (permission.Output, error) {
	connection, err := c.connect(ctx, in.Environment, true)
	if err != nil {
		return permission.Output{}, remoteSetupError("admin.permission.delete", in.Environment, in.Site, connection.environment, err)
	}
	in.Environment, in.Site = connection.environment.Alias, connection.environment.SiteContentURL
	if in.PrincipalUsername != "" {
		user, resolveErr := connection.adapter.ResolveUser(ctx, resourceadmin.UserSelector{Username: in.PrincipalUsername})
		if resolveErr != nil {
			return permission.Output{}, adminActionError("admin.permission.delete", in.Environment, in.Site, resolveErr)
		}
		in.PrincipalLUID, in.PrincipalUsername = user.LUID, ""
	}
	adapter := adminPermissionMutationAdapter{connection.adapter}
	out, err := permission.Delete(ctx, adapter, adapter, in, preview)
	return out, permissionMutationError("admin.permission.delete", in.Environment, in.Site, err)
}

func permissionMutationError(operation, environment, site string, err error) error {
	if err == nil {
		return nil
	}
	resolution, ok := errors.AsType[*resourceadmin.PrincipalResolutionError](err)
	if !ok {
		return adminActionError(operation, environment, site, err)
	}
	retryable, _ := errs.CompleteRetryAdvice(err, "Review the exact principal before retrying.")
	hint := commandhint.Environment(environment, "admin", resolution.PrincipalType, "inspect", "--id", resolution.PrincipalLUID)
	return &errs.Error{
		ID:               operation + ".principal.resolve",
		Kind:             errs.KindOperation,
		Operation:        operation,
		Resource:         resolution.PrincipalLUID,
		Environment:      environment,
		Site:             site,
		Summary:          "Permission principal resolution failed.",
		Cause:            err,
		Retryable:        retryable,
		CorrectiveAction: "Review the requested principal type and identity. Run " + hint + ", then create a new preview before any write.",
		TableauRequestID: errs.TableauRequestID(err),
		Phase:            errs.PhaseVerification,
		Outcome:          errs.OutcomeNotAttempted,
		Prerequisite:     &errs.Prerequisite{Kind: resolution.PrincipalType, Resource: resolution.PrincipalLUID, Summary: resolution.PrerequisiteSummary()},
	}
}

func (a adminPermissionMutationAdapter) DeletePermission(ctx context.Context, in permission.Input) (permission.Result, error) {
	result, err := a.adapter.DeletePermission(ctx, permissionMutationRequest(in))
	return permission.Result{Status: result.Status, ResourceLUID: result.ResourceLUID, TableauRequestID: result.RequestID}, err
}
func permissionRuleMode(set tableauadmin.PermissionSet, principalType, principalLUID, capability string) string {
	for _, rule := range set.Rules {
		if rule.PrincipalType == principalType && rule.PrincipalLUID == principalLUID && rule.Capability == capability {
			return rule.Mode
		}
	}
	return ""
}
