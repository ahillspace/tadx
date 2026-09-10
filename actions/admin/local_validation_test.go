package admin_test

import (
	"testing"

	groupcreate "github.com/ahillspace/tadx/actions/admin/group/create"
	groupdelete "github.com/ahillspace/tadx/actions/admin/group/delete"
	groupinspect "github.com/ahillspace/tadx/actions/admin/group/inspect"
	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	groupadd "github.com/ahillspace/tadx/actions/admin/group/member/add"
	groupremove "github.com/ahillspace/tadx/actions/admin/group/member/remove"
	groupupdate "github.com/ahillspace/tadx/actions/admin/group/update"
	permissioncreate "github.com/ahillspace/tadx/actions/admin/permission/create"
	permissiondelete "github.com/ahillspace/tadx/actions/admin/permission/delete"
	permissioninspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	usercreate "github.com/ahillspace/tadx/actions/admin/user/create"
	userdelete "github.com/ahillspace/tadx/actions/admin/user/delete"
	userinspect "github.com/ahillspace/tadx/actions/admin/user/inspect"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	userupdate "github.com/ahillspace/tadx/actions/admin/user/update"
)

func TestAdminLocalValidationWithoutResolvedSession(t *testing.T) {
	for name, validate := range map[string]func() error{
		"create user": func() error {
			return usercreate.ValidateInput(usercreate.Input{Environment: "selected", Name: "author", SiteRole: "Creator", AuthSetting: "ServerDefault"})
		},
		"delete user": func() error {
			return userdelete.ValidateInput(userdelete.Input{Environment: "selected", UserLUID: "u1"})
		},
		"create group": func() error {
			return groupcreate.ValidateInput(groupcreate.Input{Environment: "selected", Name: "Authors"})
		},
		"delete group": func() error {
			return groupdelete.ValidateInput(groupdelete.Input{Environment: "selected", GroupLUID: "g1"})
		},
		"add member": func() error {
			return groupadd.ValidateInput(groupadd.Input{Environment: "selected", GroupLUID: "g1", UserLUID: "u1"})
		},
		"remove member": func() error {
			return groupremove.ValidateInput(groupremove.Input{Environment: "selected", GroupLUID: "g1", UserLUID: "u1"})
		},
		"inspect user": func() error {
			return userinspect.ValidateInput(userinspect.Input{Selector: userinspect.Selector{LUID: "u1"}})
		},
		"inspect group": func() error {
			return groupinspect.ValidateInput(groupinspect.Input{Selector: groupinspect.Selector{LUID: "g1"}})
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
			return usercreate.ValidateInput(usercreate.Input{Environment: "selected", Name: "author", SiteRole: "Creator", AuthSetting: invalidAuth})
		},
		"update user auth": func() error {
			return userupdate.ValidateInput(userupdate.Input{Environment: "selected", UserLUID: "u1", AuthSetting: &invalidAuth})
		},
		"update user empty": func() error {
			return userupdate.ValidateInput(userupdate.Input{Environment: "selected", UserLUID: "u1"})
		},
		"group metadata empty": func() error {
			return groupupdate.ValidateInput(groupupdate.Input{Environment: "selected", GroupLUID: "g1"})
		},
		"duplicate desired members": func() error {
			return groupupdate.ValidateInput(groupupdate.Input{Environment: "selected", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u1", "u1"}})
		},
		"user selector conflict": func() error {
			return userinspect.ValidateInput(userinspect.Input{Selector: userinspect.Selector{LUID: "u1", Username: "author"}})
		},
		"group selector conflict": func() error {
			return groupinspect.ValidateInput(groupinspect.Input{Selector: groupinspect.Selector{LUID: "g1", Name: "Authors"}})
		},
		"user list limit":         func() error { return userlist.ValidateInput(userlist.Input{Limit: -1}) },
		"group list all conflict": func() error { return grouplist.ValidateInput(grouplist.Input{All: true, Limit: 1}) },
		"permission create":       func() error { return permissioncreate.ValidateInput(permissioncreate.Input{}) },
		"permission delete":       func() error { return permissiondelete.ValidateInput(permissiondelete.Input{}) },
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
