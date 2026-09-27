package admin

import (
	"context"
	"fmt"

	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

// PrincipalResolutionError records the exact requested principal when a
// permission mutation cannot verify its type-specific identity.
type PrincipalResolutionError struct {
	PrincipalType string
	PrincipalLUID string
	Cause         error
}

func (e *PrincipalResolutionError) Error() string {
	return fmt.Sprintf("resolve %s principal %q: %v", e.PrincipalType, e.PrincipalLUID, e.Cause)
}
func (e *PrincipalResolutionError) Unwrap() error { return e.Cause }
func (e *PrincipalResolutionError) PrerequisiteKind() string {
	return e.PrincipalType
}
func (e *PrincipalResolutionError) PrerequisiteResource() string {
	return e.PrincipalLUID
}
func (e *PrincipalResolutionError) PrerequisiteSummary() string {
	return "The requested permission principal must exist with the selected type."
}

// GetPermissionRule validates exact principal identity and reads live resource rules.
// The caller selects a capability from this snapshot without calculating effective access.
func (a *Adapter) GetPermissionRule(ctx context.Context, in tableau.PermissionMutationRequest) (tableau.PermissionSet, error) {
	if err := a.authorize("admin.permission.inspect"); err != nil {
		return tableau.PermissionSet{}, err
	}
	if err := a.resolvePermissionPrincipal(ctx, in.Rule.PrincipalType, in.Rule.PrincipalLUID); err != nil {
		return tableau.PermissionSet{}, err
	}
	return a.client.GetPermissions(ctx, in.PermissionRequest)
}

func (a *Adapter) resolvePermissionPrincipal(ctx context.Context, principalType, principalLUID string) error {
	if principalType == "user" {
		if _, err := a.ResolveUser(ctx, UserSelector{LUID: principalLUID}); err != nil {
			return &PrincipalResolutionError{PrincipalType: principalType, PrincipalLUID: principalLUID, Cause: err}
		}
		return nil
	}
	if err := a.authorize("admin.group.inspect"); err != nil {
		return err
	}
	_, err := a.client.ListGroupUsers(ctx, principalLUID, tableau.PageRequest{PageNumber: 1, PageSize: 1})
	if err != nil {
		return &PrincipalResolutionError{PrincipalType: principalType, PrincipalLUID: principalLUID, Cause: err}
	}
	return nil
}

func (a *Adapter) CreatePermission(ctx context.Context, in tableau.PermissionMutationRequest) (tableau.MutationResult, error) {
	if err := a.authorize("admin.permission.create"); err != nil {
		return tableau.MutationResult{}, err
	}
	return a.client.CreatePermission(ctx, in)
}
func (a *Adapter) DeletePermission(ctx context.Context, in tableau.PermissionMutationRequest) (tableau.MutationResult, error) {
	if err := a.authorize("admin.permission.delete"); err != nil {
		return tableau.MutationResult{}, err
	}
	return a.client.DeletePermission(ctx, in)
}
