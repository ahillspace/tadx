package admin_test

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"testing"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	groupmember "github.com/ahillspace/tadx/actions/admin/group/member"
	permission "github.com/ahillspace/tadx/actions/admin/permission"
	permissioninspect "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	cli "github.com/ahillspace/tadx/internal/cli/admin"
	"github.com/spf13/cobra"
)

type fake struct {
	userInspect  userops.InspectInput
	rendered     int
	userCreate   userops.CreateInput
	userPreview  bool
	groupUpdate  groupops.UpdateInput
	groupPreview bool
	memberAdd    groupmember.Input
	memberRemove groupmember.Input
	permissions  []permission.Input
}

func (f *fake) Render(any) error { f.rendered++; return nil }
func (f *fake) ListAdminUsers(context.Context, userops.ListInput) (userops.ListOutput, error) {
	return userops.ListOutput{}, nil
}
func (f *fake) InspectAdminUser(_ context.Context, input userops.InspectInput) (userops.InspectOutput, error) {
	f.userInspect = input
	return userops.InspectOutput{}, nil
}

func TestUserInspectUsernameAliasAndConflicts(t *testing.T) {
	for _, tail := range [][]string{{"--username", "exact-login"}, {"--username", "exact-login", "--name", "exact-login"}, {"--username", "exact-login", "--id", "user-id"}} {
		f := &fake{}
		cmd := cli.New(deps(f))
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		cmd.SetArgs(append([]string{"user", "inspect"}, tail...))
		err := cmd.ExecuteContext(t.Context())
		if len(tail) == 2 {
			if err != nil || f.userInspect.Selector.Username != "exact-login" || f.rendered != 1 {
				t.Fatalf("input=%+v err=%v", f.userInspect, err)
			}
		} else if err == nil || f.rendered != 0 {
			t.Fatalf("conflicting selectors executed: %v", tail)
		}
	}
}
func (f *fake) CreateAdminUser(_ context.Context, in userops.CreateInput, preview bool) (userops.CreateOutput, error) {
	f.userCreate = in
	f.userPreview = preview
	return userops.CreateOutput{}, nil
}
func (f *fake) UpdateAdminUser(context.Context, userops.UpdateInput, bool) (userops.UpdateOutput, error) {
	return userops.UpdateOutput{}, nil
}
func (f *fake) DeleteAdminUser(context.Context, userops.DeleteInput, bool) (userops.DeleteOutput, error) {
	return userops.DeleteOutput{}, nil
}
func (f *fake) ListAdminGroups(context.Context, groupops.ListInput) (groupops.ListOutput, error) {
	return groupops.ListOutput{}, nil
}
func (f *fake) InspectAdminGroup(context.Context, groupops.InspectInput) (groupops.InspectOutput, error) {
	return groupops.InspectOutput{}, nil
}
func (f *fake) CreateAdminGroup(context.Context, groupops.CreateInput, bool) (groupops.CreateOutput, error) {
	return groupops.CreateOutput{}, nil
}
func (f *fake) UpdateAdminGroup(_ context.Context, in groupops.UpdateInput, preview bool) (groupops.UpdateOutput, error) {
	f.groupUpdate = in
	f.groupPreview = preview
	return groupops.UpdateOutput{}, nil
}
func (f *fake) DeleteAdminGroup(context.Context, groupops.DeleteInput, bool) (groupops.DeleteOutput, error) {
	return groupops.DeleteOutput{}, nil
}
func (f *fake) AddAdminGroupMember(_ context.Context, input groupmember.Input, _ bool) (groupmember.Output, error) {
	f.memberAdd = input
	return groupmember.Output{}, nil
}
func (f *fake) RemoveAdminGroupMember(_ context.Context, input groupmember.Input, _ bool) (groupmember.Output, error) {
	f.memberRemove = input
	return groupmember.Output{}, nil
}
func (f *fake) InspectAdminPermission(context.Context, permissioninspect.Input) (permissioninspect.Output, error) {
	return permissioninspect.Output{}, nil
}
func (f *fake) CreateAdminPermission(_ context.Context, in permission.Input, _ bool) (permission.Output, error) {
	f.permissions = append(f.permissions, in)
	return permission.Output{}, nil
}
func (f *fake) DeleteAdminPermission(context.Context, permission.Input, bool) (permission.Output, error) {
	return permission.Output{}, nil
}

func deps(f *fake) cli.Dependencies {
	return cli.Dependencies{UserLister: f, UserInspector: f, UserCreator: f, UserUpdater: f, UserDeleter: f, GroupLister: f, GroupInspector: f, GroupCreator: f, GroupUpdater: f, GroupDeleter: f, GroupMemberAdder: f, GroupMemberRemover: f, PermissionInspector: f, PermissionCreator: f, PermissionDeleter: f, Renderer: f}
}

func TestCommandMountsAllCapabilitiesAndShowsMutations(t *testing.T) {
	f := &fake{}
	root := cli.New(deps(f))
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
	cmd := cli.New(deps(f))
	cmd.SetArgs([]string{"group-member", "add", "--environment", "dev", "--group-id", "g1", "--user-id", "u1", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"group-member", "remove", "--environment", "dev", "--group-id", "g1", "--user-id", "u2"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.memberAdd.GroupLUID != "g1" || f.memberAdd.UserLUID != "u1" || f.memberRemove.UserLUID != "u2" {
		t.Fatalf("add=%#v remove=%#v", f.memberAdd, f.memberRemove)
	}
}

func TestUserCreateRequiresEnvironmentAndSupportsPreview(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f))
	cmd.SetArgs([]string{"user", "create", "--name", "alex@example.com", "--site-role", "Viewer", "--auth-setting", "SAML"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--environment") {
		t.Fatalf("missing environment error = %v", err)
	}
	f = &fake{}
	cmd = cli.New(deps(f))
	cmd.SetArgs([]string{"user", "create", "--environment", "prod", "--name", "alex@example.com", "--site-role", "Viewer", "--auth-setting", "SAML"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.userPreview || f.userCreate.Environment != "prod" || f.rendered != 1 {
		t.Fatalf("input = %#v preview=%v rendered=%d", f.userCreate, f.userPreview, f.rendered)
	}
	f = &fake{}
	cmd = cli.New(deps(f))
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
	cmd := cli.New(deps(&fake{}))
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
	cmd := cli.New(deps(f))
	cmd.SetArgs([]string{"group", "update", "--environment", "prod", "--id", "g1", "--member-id", "u1"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--set-members") {
		t.Fatalf("member without set error = %v", err)
	}
	f = &fake{}
	cmd = cli.New(deps(f))
	cmd.SetArgs([]string{"group", "update", "--environment", "prod", "--id", "g1", "--set-members", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !f.groupUpdate.MembershipSet || len(f.groupUpdate.DesiredMemberLUIDs) != 0 || !f.groupPreview {
		t.Fatalf("input = %#v preview=%v", f.groupUpdate, f.groupPreview)
	}
}

func TestGroupRenameUsesNewNameAndProtectsAliasConflict(t *testing.T) {
	for _, flag := range []string{"--new-name", "--name"} {
		f := &fake{}
		cmd := cli.New(deps(f))
		cmd.SetArgs([]string{"group", "update", "--environment", "prod", "--id", "group", "--preview", flag, "Renamed"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if f.groupUpdate.Name == nil || *f.groupUpdate.Name != "Renamed" {
			t.Fatalf("input=%#v", f.groupUpdate)
		}
	}
	f := &fake{}
	cmd := cli.New(deps(f))
	cmd.SetArgs([]string{"group", "update", "--environment", "prod", "--id", "group", "--preview", "--new-name", "One", "--name", "Two"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("conflicting rename aliases accepted")
	}
}

func TestExactInspectSelectorsRejectMultipleSelectors(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f))
	cmd.SetArgs([]string{"user", "inspect", "--id", "u1", "--name", "alex"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected selector error")
	}
}

func TestUserMutationAcceptsExactUsernameSelector(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f))
	cmd.SetArgs([]string{"user", "update", "--environment", "prod", "--username", "alex@example.com", "--site-role", "Viewer", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"user", "delete", "--environment", "prod", "--username", "alex@example.com", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"user", "delete", "--environment", "prod", "--id", "u1", "--username", "alex@example.com"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("accepted both user selectors")
	}
}

func TestPermissionCreateRepeatsCapabilitiesSequentially(t *testing.T) {
	f := &fake{}
	cmd := cli.New(deps(f))
	cmd.SetArgs([]string{"permission", "create", "--environment", "prod", "--kind", "workbook", "--id", "w1", "--principal-type", "group", "--principal-id", "g1", "--mode", "Allow", "--capability", "Read", "--capability", "Write", "--preview"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(f.permissions) != 2 || f.permissions[0].Capability != "Read" || f.permissions[1].Capability != "Write" {
		t.Fatalf("permissions = %#v", f.permissions)
	}
	f = &fake{}
	cmd = cli.New(deps(f))
	cmd.SetArgs([]string{"permission", "create", "--environment", "prod", "--kind", "workbook", "--id", "w1", "--principal-type", "group", "--principal-id", "g1", "--mode", "Allow", "--capability", "Read", "--capability", "Read"})
	if err := cmd.Execute(); err == nil || len(f.permissions) != 0 {
		t.Fatalf("duplicate capability result = %v calls=%d", err, len(f.permissions))
	}
}

func TestPermissionCapabilitiesValidateEntireSelectionBeforeWork(t *testing.T) {
	f := &fake{}
	d := deps(f)
	d.PermissionCapabilities = func(kind string) []string {
		if kind == "workbook" {
			return []string{"Read"}
		}
		return nil
	}
	cmd := cli.New(d)
	cmd.SetArgs([]string{"permission", "create", "--environment", "prod", "--kind", "workbook", "--id", "w1", "--principal-type", "group", "--principal-id", "g1", "--mode", "Allow", "--capability", "Read", "--capability", "Write"})
	if err := cmd.Execute(); err == nil || len(f.permissions) != 0 {
		t.Fatalf("invalid later capability err=%v calls=%d", err, len(f.permissions))
	}
}
