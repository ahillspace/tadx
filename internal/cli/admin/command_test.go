package admin_test

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"testing"

	groupcreate "github.com/ahillspace/tadx/actions/admin/group/create"
	groupdelete "github.com/ahillspace/tadx/actions/admin/group/delete"
	groupinspect "github.com/ahillspace/tadx/actions/admin/group/inspect"
	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	groupmemberadd "github.com/ahillspace/tadx/actions/admin/group/member/add"
	groupmemberremove "github.com/ahillspace/tadx/actions/admin/group/member/remove"
	groupupdate "github.com/ahillspace/tadx/actions/admin/group/update"
	permissioninspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	usercreate "github.com/ahillspace/tadx/actions/admin/user/create"
	userdelete "github.com/ahillspace/tadx/actions/admin/user/delete"
	userinspect "github.com/ahillspace/tadx/actions/admin/user/inspect"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	userupdate "github.com/ahillspace/tadx/actions/admin/user/update"
	cli "github.com/ahillspace/tadx/internal/cli/admin"
	"github.com/spf13/cobra"
)

type fake struct {
	rendered     int
	userCreate   usercreate.Input
	userPreview  bool
	groupUpdate  groupupdate.Input
	groupPreview bool
	memberAdd    groupmemberadd.Input
	memberRemove groupmemberremove.Input
}

func (f *fake) Render(any) error { f.rendered++; return nil }
func (f *fake) ListAdminUsers(context.Context, userlist.Input) (userlist.Output, error) {
	return userlist.Output{}, nil
}
func (f *fake) InspectAdminUser(context.Context, userinspect.Input) (userinspect.Output, error) {
	return userinspect.Output{}, nil
}
func (f *fake) CreateAdminUser(_ context.Context, in usercreate.Input, preview bool) (usercreate.Output, error) {
	f.userCreate = in
	f.userPreview = preview
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
func (f *fake) InspectAdminGroup(context.Context, groupinspect.Input) (groupinspect.Output, error) {
	return groupinspect.Output{}, nil
}
func (f *fake) CreateAdminGroup(context.Context, groupcreate.Input, bool) (groupcreate.Output, error) {
	return groupcreate.Output{}, nil
}
func (f *fake) UpdateAdminGroup(_ context.Context, in groupupdate.Input, preview bool) (groupupdate.Output, error) {
	f.groupUpdate = in
	f.groupPreview = preview
	return groupupdate.Output{}, nil
}
func (f *fake) DeleteAdminGroup(context.Context, groupdelete.Input, bool) (groupdelete.Output, error) {
	return groupdelete.Output{}, nil
}
func (f *fake) AddAdminGroupMember(_ context.Context, input groupmemberadd.Input, _ bool) (groupmemberadd.Output, error) {
	f.memberAdd = input
	return groupmemberadd.Output{}, nil
}
func (f *fake) RemoveAdminGroupMember(_ context.Context, input groupmemberremove.Input, _ bool) (groupmemberremove.Output, error) {
	f.memberRemove = input
	return groupmemberremove.Output{}, nil
}
func (f *fake) InspectAdminPermission(context.Context, permissioninspect.Input) (permissioninspect.Output, error) {
	return permissioninspect.Output{}, nil
}

func deps(f *fake, enabled bool) cli.Dependencies {
	return cli.Dependencies{UserLister: f, UserInspector: f, UserCreator: f, UserUpdater: f, UserDeleter: f, GroupLister: f, GroupInspector: f, GroupCreator: f, GroupUpdater: f, GroupDeleter: f, GroupMemberAdder: f, GroupMemberRemover: f, PermissionInspector: f, Renderer: f, MutationsEnabled: enabled}
}

func TestCommandMountsAllCapabilitiesAndShowsMutations(t *testing.T) {
	f := &fake{}
	root := cli.New(deps(f, false))
	var got []string
	var walk func(*cobra.Command)
	walk = func(parent *cobra.Command) {
		for _, child := range parent.Commands() {
			if id := child.Annotations["tadx.capability"]; id != "" {
				got = append(got, id)
			}
			mutation := strings.HasSuffix(child.Name(), "create") || strings.HasSuffix(child.Name(), "update") || strings.HasSuffix(child.Name(), "delete") || child.Name() == "add" || child.Name() == "remove"
			if mutation && child.Hidden {
				t.Errorf("%s is hidden", child.CommandPath())
			}
			walk(child)
		}
	}
	walk(root)
	sort.Strings(got)
	want := []string{"admin.group.create", "admin.group.delete", "admin.group.inspect", "admin.group.list", "admin.group.member.add", "admin.group.member.remove", "admin.group.update", "admin.permission.create", "admin.permission.delete", "admin.permission.inspect", "admin.user.create", "admin.user.delete", "admin.user.inspect", "admin.user.list", "admin.user.update"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("capabilities = %v", got)
	}
}

func TestGroupMemberCommandsMapExactIdentities(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f, true))
	cmd.SetArgs([]string{"group", "member", "add", "--environment", "dev", "--group-id", "g1", "--user-id", "u1", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"group", "member", "remove", "--environment", "dev", "--group-id", "g1", "--user-id", "u2"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.memberAdd.GroupLUID != "g1" || f.memberAdd.UserLUID != "u1" || f.memberRemove.UserLUID != "u2" {
		t.Fatalf("add=%#v remove=%#v", f.memberAdd, f.memberRemove)
	}
}

func TestUserCreateRequiresEnvironmentAndSupportsPreview(t *testing.T) {
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
	if f.userPreview || f.userCreate.Environment != "prod" || f.rendered != 1 {
		t.Fatalf("input = %#v preview=%v rendered=%d", f.userCreate, f.userPreview, f.rendered)
	}
	f = &fake{}
	cmd = cli.New(deps(f, true))
	cmd.SetArgs([]string{"user", "create", "--environment", "prod", "--name", "alex@example.com", "--site-role", "Viewer", "--auth-setting", "SAML", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !f.userPreview {
		t.Fatal("--preview was not delegated")
	}
}

func TestUserCreateHelpExplainsAuthenticationSelectorRule(t *testing.T) {
	var stdout bytes.Buffer
	cmd := cli.New(deps(&fake{}, true))
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"user", "create", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "exactly one of --auth-setting or --idp-configuration-id") {
		t.Fatalf("help = %q", stdout.String())
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
	cmd.SetArgs([]string{"group", "update", "--environment", "prod", "--id", "g1", "--set-members", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !f.groupUpdate.MembershipSet || len(f.groupUpdate.DesiredMemberLUIDs) != 0 || !f.groupPreview {
		t.Fatalf("input = %#v preview=%v", f.groupUpdate, f.groupPreview)
	}
}

func TestExactInspectSelectorsRejectMultipleSelectors(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f, true))
	cmd.SetArgs([]string{"user", "inspect", "--id", "u1", "--name", "alex"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected selector error")
	}
}
