package admin_test

import (
	"context"
	"errors"
	"testing"

	resource "github.com/ahillspace/tadx/internal/resources/admin"
	tableau "github.com/ahillspace/tadx/internal/tableau/admin"
)

type permissionClient struct {
	tableau.ClientContract
	userCalls, groupCalls, memberCalls, permissionCalls, writes int
	missing                                                     bool
}

func (f *permissionClient) GetUser(_ context.Context, id string) (tableau.User, error) {
	f.userCalls++
	if f.missing {
		return tableau.User{}, errors.New("user not found")
	}
	return tableau.User{LUID: id, Name: "test-user"}, nil
}
func (f *permissionClient) ListGroups(_ context.Context, in tableau.ListGroupsRequest) (tableau.GroupPage, error) {
	f.groupCalls++
	return tableau.GroupPage{Number: 1, Size: in.PageSize, Total: 1, Items: []tableau.Group{{LUID: "g1", Name: "Test group"}}}, nil
}
func (f *permissionClient) ListGroupUsers(_ context.Context, id string, in tableau.PageRequest) (tableau.UserPage, error) {
	f.memberCalls++
	if f.missing || id != "g1" {
		return tableau.UserPage{}, errors.New("group not found")
	}
	return tableau.UserPage{Number: 1, Size: in.PageSize, Total: 0}, nil
}
func (f *permissionClient) GetPermissions(_ context.Context, in tableau.PermissionRequest) (tableau.PermissionSet, error) {
	f.permissionCalls++
	return tableau.PermissionSet{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, Source: "direct"}, nil
}
func (f *permissionClient) CreatePermission(_ context.Context, in tableau.PermissionMutationRequest) (tableau.MutationResult, error) {
	f.writes++
	return tableau.MutationResult{Status: "created", ResourceLUID: in.ResourceLUID}, nil
}
func (f *permissionClient) DeletePermission(_ context.Context, in tableau.PermissionMutationRequest) (tableau.MutationResult, error) {
	f.writes++
	return tableau.MutationResult{Status: "deleted", ResourceLUID: in.ResourceLUID}, nil
}
func TestPermissionMutationReadValidatesPrincipalBeforeResource(t *testing.T) {
	for _, principal := range []string{"user", "group"} {
		f := &permissionClient{}
		adapter := resource.NewAdapter(f)
		id := "u1"
		if principal == "group" {
			id = "g1"
		}
		in := tableau.PermissionMutationRequest{PermissionRequest: tableau.PermissionRequest{ResourceKind: "workbook", ResourceLUID: "w1"}, Rule: tableau.PermissionRule{PrincipalType: principal, PrincipalLUID: id, Capability: "Read", Mode: "Allow"}}
		set, err := adapter.GetPermissionRule(context.Background(), in)
		if err != nil || set.ResourceLUID != "w1" || f.permissionCalls != 1 || f.userCalls+f.memberCalls != 1 || f.groupCalls != 0 || f.writes != 0 {
			t.Fatalf("set=%+v err=%v fake=%+v", set, err, f)
		}
		if _, err := adapter.CreatePermission(context.Background(), in); err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.DeletePermission(context.Background(), in); err != nil || f.writes != 2 {
			t.Fatalf("writes=%d err=%v", f.writes, err)
		}
	}
}
func TestPermissionMutationReadRejectsInvalidOrMissingPrincipal(t *testing.T) {
	for _, missing := range []bool{true, false} {
		f := &permissionClient{missing: missing}
		in := tableau.PermissionMutationRequest{PermissionRequest: tableau.PermissionRequest{ResourceKind: "workbook", ResourceLUID: "w1"}, Rule: tableau.PermissionRule{PrincipalType: "user", PrincipalLUID: "u1", Capability: "Read", Mode: "Allow"}}
		if !missing {
			in.Rule.Capability = "MadeUp"
		}
		_, err := resource.NewAdapter(f).GetPermissionRule(context.Background(), in)
		if err == nil || f.permissionCalls != 0 || f.writes != 0 {
			t.Fatalf("err=%v fake=%+v", err, f)
		}
	}
}
