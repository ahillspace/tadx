package admin

import (
	"context"
	"errors"

	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

// GetPermissionRule validates exact principal identity and reads live resource rules.
// The caller selects a capability from this snapshot without calculating effective access.
func (a *Adapter) GetPermissionRule(ctx context.Context, in tableau.PermissionMutationRequest) (tableau.PermissionSet, error) {
	if err := a.configured(); err != nil {
		return tableau.PermissionSet{}, err
	}
	if err := tableau.ValidatePermissionMutation(in); err != nil {
		return tableau.PermissionSet{}, err
	}
	if in.Rule.PrincipalType == "user" {
		if _, err := a.ResolveUser(ctx, UserSelector{LUID: in.Rule.PrincipalLUID}); err != nil {
			return tableau.PermissionSet{}, err
		}
	} else {
		if _, err := a.ResolveGroup(ctx, GroupSelector{LUID: in.Rule.PrincipalLUID}, false); err != nil {
			return tableau.PermissionSet{}, err
		}
	}
	set, err := a.GetPermissions(ctx, in.PermissionRequest)
	if err != nil {
		return tableau.PermissionSet{}, err
	}
	if set.ResourceKind != in.ResourceKind || set.ResourceLUID != in.ResourceLUID {
		return tableau.PermissionSet{}, errors.New("permission reader returned a different authoritative resource identity")
	}
	return set, nil
}

func (a *Adapter) CreatePermission(ctx context.Context, in tableau.PermissionMutationRequest) (tableau.MutationResult, error) {
	if err := a.configured(); err != nil {
		return tableau.MutationResult{}, err
	}
	writer, ok := a.client.(tableau.PermissionMutationClient)
	if !ok {
		return tableau.MutationResult{}, errors.New("permission mutation client is not configured")
	}
	return writer.CreatePermission(ctx, in)
}
func (a *Adapter) DeletePermission(ctx context.Context, in tableau.PermissionMutationRequest) (tableau.MutationResult, error) {
	if err := a.configured(); err != nil {
		return tableau.MutationResult{}, err
	}
	writer, ok := a.client.(tableau.PermissionMutationClient)
	if !ok {
		return tableau.MutationResult{}, errors.New("permission mutation client is not configured")
	}
	return writer.DeletePermission(ctx, in)
}
