package admin_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	groupcreate "github.com/ahillspace/tadx/actions/admin/group/create"
	groupdelete "github.com/ahillspace/tadx/actions/admin/group/delete"
	groupget "github.com/ahillspace/tadx/actions/admin/group/get"
	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	groupupdate "github.com/ahillspace/tadx/actions/admin/group/update"
	permissionget "github.com/ahillspace/tadx/actions/admin/permission/get"
	usercreate "github.com/ahillspace/tadx/actions/admin/user/create"
	userdelete "github.com/ahillspace/tadx/actions/admin/user/delete"
	userget "github.com/ahillspace/tadx/actions/admin/user/get"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	userupdate "github.com/ahillspace/tadx/actions/admin/user/update"
	cli "github.com/ahillspace/tadx/internal/cli/admin"
)

type fake struct {
	rendered    int
	userCreate  usercreate.Input
	userApply   bool
	groupUpdate groupupdate.Input
	groupApply  bool
}

func (f *fake) Render(any) error { f.rendered++; return nil }
func (f *fake) ListAdminUsers(context.Context, userlist.Input) (userlist.Output, error) {
	return userlist.Output{}, nil
}
func (f *fake) GetAdminUser(context.Context, userget.Input) (userget.Output, error) {
	return userget.Output{}, nil
}
func (f *fake) CreateAdminUser(_ context.Context, in usercreate.Input, apply bool) (usercreate.Output, error) {
	f.userCreate = in
	f.userApply = apply
	return usercreate.Output{}, nil
}
func (f *fake) UpdateAdminUser(context.Context, userupdate.Input, bool) (userupdate.Output, error) {
	return userupdate.Output{}, nil
}
func (f *fake) DeleteAdminUser(context.Context, userdelete.Input, bool) (userdelete.Output, error) {
	return userdelete.Output{}, nil
}
func (f *fake) ListAdminGroups(context.Context, grouplist.Input) (grouplist.Output, error) {
	return grouplist.Output{}, nil
}
func (f *fake) GetAdminGroup(context.Context, groupget.Input) (groupget.Output, error) {
	return groupget.Output{}, nil
}
func (f *fake) CreateAdminGroup(context.Context, groupcreate.Input, bool) (groupcreate.Output, error) {
	return groupcreate.Output{}, nil
}
func (f *fake) UpdateAdminGroup(_ context.Context, in groupupdate.Input, apply bool) (groupupdate.Output, error) {
	f.groupUpdate = in
	f.groupApply = apply
	return groupupdate.Output{}, nil
}
func (f *fake) DeleteAdminGroup(context.Context, groupdelete.Input, bool) (groupdelete.Output, error) {
	return groupdelete.Output{}, nil
}
func (f *fake) GetAdminPermission(context.Context, permissionget.Input) (permissionget.Output, error) {
	return permissionget.Output{}, nil
}

func deps(f *fake, enabled bool) cli.Dependencies {
	return cli.Dependencies{UserLister: f, UserGetter: f, UserCreator: f, UserUpdater: f, UserDeleter: f, GroupLister: f, GroupGetter: f, GroupCreator: f, GroupUpdater: f, GroupDeleter: f, PermissionGetter: f, Renderer: f, MutationsEnabled: enabled}
}

func TestCommandMountsAllCapabilitiesAndHidesMutationDiscovery(t *testing.T) {
	f := &fake{}
	root := cli.New(deps(f, false))
	var got []string
	for _, parent := range root.Commands() {
		for _, child := range parent.Commands() {
			got = append(got, child.Annotations["tadx.capability"])
			mutation := strings.HasSuffix(child.Name(), "create") || strings.HasSuffix(child.Name(), "update") || strings.HasSuffix(child.Name(), "delete")
			if mutation && !child.Hidden {
				t.Errorf("%s is visible", child.CommandPath())
			}
		}
	}
	sort.Strings(got)
	want := []string{"admin.group.create", "admin.group.delete", "admin.group.get", "admin.group.list", "admin.group.update", "admin.permission.get", "admin.user.create", "admin.user.delete", "admin.user.get", "admin.user.list", "admin.user.update"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("capabilities = %v", got)
	}
}

func TestUserCreateRequiresEnvironmentAndPreservesPreviewApply(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f, true))
	cmd.SetArgs([]string{"user", "create", "--name", "alex@example.com", "--site-role", "Viewer", "--auth-setting", "SAML"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--environment") {
		t.Fatalf("missing environment error = %v", err)
	}
	f = &fake{}
	cmd = cli.New(deps(f, true))
	cmd.SetArgs([]string{"user", "create", "--environment", "prod", "--name", "alex@example.com", "--site-role", "Viewer", "--auth-setting", "SAML"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.userApply || f.userCreate.Environment != "prod" || f.rendered != 1 {
		t.Fatalf("preview = %#v apply=%v rendered=%d", f.userCreate, f.userApply, f.rendered)
	}
	f = &fake{}
	cmd = cli.New(deps(f, true))
	cmd.SetArgs([]string{"user", "create", "--environment", "prod", "--name", "alex@example.com", "--site-role", "Viewer", "--auth-setting", "SAML", "--apply"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !f.userApply {
		t.Fatal("--apply was not delegated")
	}
}

func TestGroupUpdateDistinguishesOmittedAndExplicitEmptyMembership(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f, true))
	cmd.SetArgs([]string{"group", "update", "--environment", "prod", "--id", "g1", "--member-id", "u1"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--set-members") {
		t.Fatalf("member without set error = %v", err)
	}
	f = &fake{}
	cmd = cli.New(deps(f, true))
	cmd.SetArgs([]string{"group", "update", "--environment", "prod", "--id", "g1", "--set-members", "--apply"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !f.groupUpdate.MembershipSet || len(f.groupUpdate.DesiredMemberLUIDs) != 0 || !f.groupApply {
		t.Fatalf("input = %#v apply=%v", f.groupUpdate, f.groupApply)
	}
}

func TestExactGetSelectorsRejectMultipleSelectors(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f, true))
	cmd.SetArgs([]string{"user", "get", "--id", "u1", "--name", "alex"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected selector error")
	}
}
