package admin_test

import (
	"testing"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	groupmember "github.com/ahillspace/tadx/actions/admin/group/member"
	permission "github.com/ahillspace/tadx/actions/admin/permission"
	permissioninspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	userops "github.com/ahillspace/tadx/actions/admin/user"
)

func TestAdminLocalValidationWithoutResolvedSession(t *testing.T) {
	for name, validate := range map[string]func() error{
		"create user": func() error {
			return userops.ValidateCreateInput(userops.CreateInput{Environment: "selected", Name: "author", SiteRole: "Creator", AuthSetting: "ServerDefault"})
		},
		"delete user": func() error {
			return userops.ValidateDeleteInput(userops.DeleteInput{Environment: "selected", UserLUID: "u1"})
		},
		"create group": func() error {
			return groupops.ValidateCreateInput(groupops.CreateInput{Environment: "selected", Name: "Authors"})
		},
		"delete group": func() error {
			return groupops.ValidateDeleteInput(groupops.DeleteInput{Environment: "selected", GroupLUID: "g1"})
		},
		"add member": func() error {
			return groupmember.ValidateAddInput(groupmember.Input{Environment: "selected", GroupLUID: "g1", UserLUID: "u1"})
		},
		"remove member": func() error {
			return groupmember.ValidateRemoveInput(groupmember.Input{Environment: "selected", GroupLUID: "g1", UserLUID: "u1"})
		},
		"inspect user": func() error {
			return userops.ValidateInspectInput(userops.InspectInput{Selector: userops.Selector{LUID: "u1"}})
		},
		"inspect group": func() error {
			return groupops.ValidateInspectInput(groupops.InspectInput{Selector: groupops.Selector{LUID: "g1"}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validate(); err != nil {
				t.Fatalf("valid raw input required a fake site/session: %v", err)
			}
		})
	}
}

func TestAdminLocalValidationRejectsInvalidRequests(t *testing.T) {
	invalidAuth := "Creator"
	for name, validate := range map[string]func() error{
		"create user auth": func() error {
			return userops.ValidateCreateInput(userops.CreateInput{Environment: "selected", Name: "author", SiteRole: "Creator", AuthSetting: invalidAuth})
		},
		"update user auth": func() error {
			return userops.ValidateUpdateInput(userops.UpdateInput{Environment: "selected", UserLUID: "u1", AuthSetting: &invalidAuth})
		},
		"update user empty": func() error {
			return userops.ValidateUpdateInput(userops.UpdateInput{Environment: "selected", UserLUID: "u1"})
		},
		"group metadata empty": func() error {
			return groupops.ValidateUpdateInput(&groupops.UpdateInput{Environment: "selected", GroupLUID: "g1"})
		},
		"duplicate desired members": func() error {
			return groupops.ValidateUpdateInput(&groupops.UpdateInput{Environment: "selected", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u1", "u1"}})
		},
		"user selector conflict": func() error {
			return userops.ValidateInspectInput(userops.InspectInput{Selector: userops.Selector{LUID: "u1", Username: "author"}})
		},
		"group selector conflict": func() error {
			return groupops.ValidateInspectInput(groupops.InspectInput{Selector: groupops.Selector{LUID: "g1", Name: "Authors"}})
		},
		"user list limit":         func() error { return userops.ValidateListInput(&userops.ListInput{Limit: -1}) },
		"group list all conflict": func() error { return groupops.ValidateListInput(&groupops.ListInput{All: true, Limit: 1}) },
		"permission create":       func() error { return permission.ValidateCreateInput(permission.Input{}) },
		"permission delete":       func() error { return permission.ValidateDeleteInput(permission.Input{}) },
		"permission inspect kind": func() error {
			return permissioninspect.ValidateInput(permissioninspect.Input{ResourceKind: "unknown", ResourceLUID: "r1"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validate(); err == nil {
				t.Fatal("invalid raw input accepted")
			}
		})
	}
}
