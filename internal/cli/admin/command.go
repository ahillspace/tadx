// Package admin contains thin Cobra plumbing for Tableau administration commands.
package admin

import (
	"context"
	"errors"

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
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type Renderer interface{ Render(any) error }
type UserLister interface {
	ListAdminUsers(context.Context, userlist.Input) (userlist.Output, error)
}
type UserInspector interface {
	InspectAdminUser(context.Context, userinspect.Input) (userinspect.Output, error)
}
type UserCreator interface {
	CreateAdminUser(context.Context, usercreate.Input, bool) (usercreate.Output, error)
}
type UserUpdater interface {
	UpdateAdminUser(context.Context, userupdate.Input, bool) (userupdate.Output, error)
}
type UserDeleter interface {
	DeleteAdminUser(context.Context, userdelete.Input, bool) (userdelete.Output, error)
}
type GroupLister interface {
	ListAdminGroups(context.Context, grouplist.Input) (grouplist.Output, error)
}
type GroupInspector interface {
	InspectAdminGroup(context.Context, groupinspect.Input) (groupinspect.Output, error)
}
type GroupCreator interface {
	CreateAdminGroup(context.Context, groupcreate.Input, bool) (groupcreate.Output, error)
}
type GroupUpdater interface {
	UpdateAdminGroup(context.Context, groupupdate.Input, bool) (groupupdate.Output, error)
}
type GroupDeleter interface {
	DeleteAdminGroup(context.Context, groupdelete.Input, bool) (groupdelete.Output, error)
}
type GroupMemberAdder interface {
	AddAdminGroupMember(context.Context, groupmemberadd.Input, bool) (groupmemberadd.Output, error)
}
type GroupMemberRemover interface {
	RemoveAdminGroupMember(context.Context, groupmemberremove.Input, bool) (groupmemberremove.Output, error)
}
type PermissionInspector interface {
	InspectAdminPermission(context.Context, permissioninspect.Input) (permissioninspect.Output, error)
}

type Dependencies struct {
	PermissionCreator   PermissionCreator
	PermissionDeleter   PermissionDeleter
	UserLister          UserLister
	UserInspector       UserInspector
	UserCreator         UserCreator
	UserUpdater         UserUpdater
	UserDeleter         UserDeleter
	GroupLister         GroupLister
	GroupInspector      GroupInspector
	GroupCreator        GroupCreator
	GroupUpdater        GroupUpdater
	GroupDeleter        GroupDeleter
	GroupMemberAdder    GroupMemberAdder
	GroupMemberRemover  GroupMemberRemover
	PermissionInspector PermissionInspector
	Renderer            Renderer
	MutationsEnabled    bool
}

func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "admin", Short: "Administer Tableau users, groups, and permissions"}
	user := &cobra.Command{Use: "user", Short: "Administer site users"}
	user.AddCommand(newUserList(deps), newUserInspect(deps), newUserCreate(deps), newUserUpdate(deps), newUserDelete(deps))
	group := &cobra.Command{Use: "group", Short: "Administer site groups"}
	group.AddCommand(newGroupList(deps), newGroupInspect(deps), newGroupCreate(deps), newGroupUpdate(deps), newGroupDelete(deps))
	member := &cobra.Command{Use: "member", Short: "Change direct group membership"}
	member.AddCommand(newGroupMemberAdd(deps), newGroupMemberRemove(deps))
	group.AddCommand(member)
	permission := &cobra.Command{Use: "permission", Short: "Manage exact permission rules"}
	permission.AddCommand(newPermissionInspect(deps), newPermissionCreate(deps), newPermissionDelete(deps))
	command.AddCommand(user, group, permission)
	return command
}

func newGroupMemberAdd(deps Dependencies) *cobra.Command {
	var in groupmemberadd.Input
	var preview bool
	cmd := mutation("add", "Add one user to one group.", "admin.group.member.add", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.member.add")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.group.member.add", in.Environment); err != nil {
			return err
		}
		if in.GroupLUID == "" || in.UserLUID == "" {
			return clierr.Usage("admin.group.member.add", errors.New("--group-id and --user-id are required"))
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.GroupMemberAdder.AddAdminGroupMember(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "group-id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&in.UserLUID, "user-id", "", "authoritative user LUID")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}

func newGroupMemberRemove(deps Dependencies) *cobra.Command {
	var in groupmemberremove.Input
	var preview bool
	cmd := mutation("remove", "Remove one user from one group.", "admin.group.member.remove", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.member.remove")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.group.member.remove", in.Environment); err != nil {
			return err
		}
		if in.GroupLUID == "" || in.UserLUID == "" {
			return clierr.Usage("admin.group.member.remove", errors.New("--group-id and --user-id are required"))
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.GroupMemberRemover.RemoveAdminGroupMember(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "group-id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&in.UserLUID, "user-id", "", "authoritative user LUID")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}

func newUserList(deps Dependencies) *cobra.Command {
	var input userlist.Input
	cmd := &cobra.Command{Use: "list", Short: "List one bounded site-user page.", Annotations: map[string]string{"tadx.capability": "admin.user.list"}, Args: noArgs("admin.user.list"), RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.UserLister.ListAdminUsers(cmd.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&input.Name, "name", "", "exact username filter")
	cmd.Flags().StringVar(&input.SiteRole, "site-role", "", "exact site-role filter")
	cmd.Flags().IntVar(&input.Limit, "limit", 0, "maximum users to return")
	cmd.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	cmd.Flags().BoolVar(&input.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return cmd
}
func newUserInspect(deps Dependencies) *cobra.Command {
	var in userinspect.Input
	var id, name string
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect one exact site user.", Annotations: map[string]string{"tadx.capability": "admin.user.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.inspect")(cmd, args); err != nil {
			return err
		}
		if (id == "") == (name == "") {
			return clierr.Usage("admin.user.inspect", errors.New("use exactly one of --id or --name"))
		}
		in.SetSelector(id, name)
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.UserInspector.InspectAdminUser(cmd.Context(), in)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&id, "id", "", "authoritative user LUID")
	cmd.Flags().StringVar(&name, "name", "", "exact username or email")
	cmd.Flags().BoolVar(&in.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return cmd
}
func newUserCreate(deps Dependencies) *cobra.Command {
	var in usercreate.Input
	var preview bool
	cmd := mutation("create", "Add one exact site user.", "admin.user.create", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.create")(cmd, args); err != nil {
			return err
		}
		return requireMutation(cmd, "admin.user.create", in.Environment)
	}, func(cmd *cobra.Command) error {
		out, err := deps.UserCreator.CreateAdminUser(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Long = "Add one exact site user.\n\nProvide exactly one of --auth-setting or --idp-configuration-id."
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact username or email")
	cmd.Flags().StringVar(&in.SiteRole, "site-role", "", "explicit site role")
	cmd.Flags().StringVar(&in.AuthSetting, "auth-setting", "", "explicit authentication setting")
	cmd.Flags().StringVar(&in.IdPConfigurationID, "idp-configuration-id", "", "explicit IdP configuration LUID")
	cmd.Flags().StringVar(&in.IdentityPoolName, "identity-pool", "", "explicit identity-pool name")
	cmd.Flags().StringVar(&in.Email, "email", "", "notification email address")
	cmd.Flags().StringVar(&in.Language, "language", "", "explicit language code")
	cmd.Flags().StringVar(&in.Locale, "locale", "", "explicit locale code")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
func newUserUpdate(deps Dependencies) *cobra.Command {
	var in userupdate.Input
	var preview bool
	var fullName, email, siteRole, auth, identityPool, idp, language, locale string
	cmd := mutation("update", "Update one exact site user.", "admin.user.update", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.update")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.user.update", in.Environment); err != nil {
			return err
		}
		if in.UserLUID == "" {
			return clierr.Usage("admin.user.update", errors.New("--id is required"))
		}
		setString(cmd, "full-name", fullName, &in.FullName)
		setString(cmd, "email", email, &in.Email)
		setString(cmd, "site-role", siteRole, &in.SiteRole)
		setString(cmd, "auth-setting", auth, &in.AuthSetting)
		setString(cmd, "identity-pool", identityPool, &in.IdentityPoolName)
		setString(cmd, "idp-configuration-id", idp, &in.IdPConfigurationID)
		setString(cmd, "language", language, &in.Language)
		setString(cmd, "locale", locale, &in.Locale)
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.UserUpdater.UpdateAdminUser(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.UserLUID, "id", "", "authoritative user LUID")
	cmd.Flags().StringVar(&fullName, "full-name", "", "explicit full name")
	cmd.Flags().StringVar(&email, "email", "", "explicit notification email")
	cmd.Flags().StringVar(&siteRole, "site-role", "", "explicit site role")
	cmd.Flags().StringVar(&auth, "auth-setting", "", "explicit authentication setting")
	cmd.Flags().StringVar(&identityPool, "identity-pool", "", "explicit identity-pool name")
	cmd.Flags().StringVar(&idp, "idp-configuration-id", "", "explicit IdP configuration LUID")
	cmd.Flags().StringVar(&language, "language", "", "explicit language code")
	cmd.Flags().StringVar(&locale, "locale", "", "explicit locale code")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
func newUserDelete(deps Dependencies) *cobra.Command {
	var in userdelete.Input
	var preview bool
	cmd := mutation("delete", "Remove one exact site user.", "admin.user.delete", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.delete")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.user.delete", in.Environment); err != nil {
			return err
		}
		if in.UserLUID == "" {
			return clierr.Usage("admin.user.delete", errors.New("--id is required"))
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.UserDeleter.DeleteAdminUser(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.UserLUID, "id", "", "authoritative user LUID")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}

func newGroupList(deps Dependencies) *cobra.Command {
	var in grouplist.Input
	cmd := &cobra.Command{Use: "list", Short: "List one bounded group page.", Annotations: map[string]string{"tadx.capability": "admin.group.list"}, Args: noArgs("admin.group.list"), RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.GroupLister.ListAdminGroups(cmd.Context(), in)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact group-name filter")
	cmd.Flags().StringVar(&in.Domain, "domain", "", "exact directory-domain filter")
	cmd.Flags().IntVar(&in.Limit, "limit", 0, "maximum groups to return")
	cmd.Flags().StringVar(&in.Cursor, "cursor", "", "opaque continuation cursor")
	cmd.Flags().BoolVar(&in.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return cmd
}
func newGroupInspect(deps Dependencies) *cobra.Command {
	var in groupinspect.Input
	var id, name string
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect one exact group.", Annotations: map[string]string{"tadx.capability": "admin.group.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.inspect")(cmd, args); err != nil {
			return err
		}
		if (id == "") == (name == "") {
			return clierr.Usage("admin.group.inspect", errors.New("use exactly one of --id or --name"))
		}
		in.SetSelector(id, name)
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.GroupInspector.InspectAdminGroup(cmd.Context(), in)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&id, "id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&name, "name", "", "exact group name")
	cmd.Flags().BoolVar(&in.IncludeMembers, "members", false, "include bounded direct membership")
	cmd.Flags().BoolVar(&in.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return cmd
}
func newGroupCreate(deps Dependencies) *cobra.Command {
	var in groupcreate.Input
	var external bool
	var preview bool
	cmd := mutation("create", "Create one exact group.", "admin.group.create", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.create")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.group.create", in.Environment); err != nil {
			return err
		}
		if cmd.Flags().Changed("external-user-enabled") {
			in.ExternalUserEnabled = &external
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.GroupCreator.CreateAdminGroup(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact group name")
	cmd.Flags().StringVar(&in.MinimumSiteRole, "minimum-site-role", "", "explicit minimum site role")
	cmd.Flags().BoolVar(&external, "external-user-enabled", false, "explicit on-demand external-user setting")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
func newGroupUpdate(deps Dependencies) *cobra.Command {
	var in groupupdate.Input
	var name, role string
	var external, setMembers, preview bool
	var members []string
	cmd := mutation("update", "Update one exact group.", "admin.group.update", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.update")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.group.update", in.Environment); err != nil {
			return err
		}
		if in.GroupLUID == "" {
			return clierr.Usage("admin.group.update", errors.New("--id is required"))
		}
		setString(cmd, "name", name, &in.Name)
		setString(cmd, "minimum-site-role", role, &in.MinimumSiteRole)
		if cmd.Flags().Changed("external-user-enabled") {
			in.ExternalUserEnabled = &external
		}
		if len(members) > 0 && !setMembers {
			return clierr.Usage("admin.group.update", errors.New("--member-id requires --set-members"))
		}
		in.MembershipSet = setMembers
		if setMembers {
			in.DesiredMemberLUIDs = append([]string(nil), members...)
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.GroupUpdater.UpdateAdminGroup(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&name, "name", "", "explicit new group name")
	cmd.Flags().StringVar(&role, "minimum-site-role", "", "explicit minimum site role")
	cmd.Flags().BoolVar(&external, "external-user-enabled", false, "explicit on-demand external-user setting")
	cmd.Flags().BoolVar(&setMembers, "set-members", false, "converge direct membership to the repeated --member-id values, including an empty set")
	cmd.Flags().StringArrayVar(&members, "member-id", nil, "authoritative desired direct-member LUID; repeat for each member")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
func newGroupDelete(deps Dependencies) *cobra.Command {
	var in groupdelete.Input
	var preview bool
	cmd := mutation("delete", "Delete one exact group.", "admin.group.delete", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.delete")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.group.delete", in.Environment); err != nil {
			return err
		}
		if in.GroupLUID == "" {
			return clierr.Usage("admin.group.delete", errors.New("--id is required"))
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.GroupDeleter.DeleteAdminGroup(cmd.Context(), in, preview)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "id", "", "authoritative group LUID")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
func newPermissionInspect(deps Dependencies) *cobra.Command {
	var in permissioninspect.Input
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect permission rules for one exact resource.", Annotations: map[string]string{"tadx.capability": "admin.permission.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.permission.inspect")(cmd, args); err != nil {
			return err
		}
		if in.ResourceKind == "" || in.ResourceLUID == "" {
			return clierr.Usage("admin.permission.inspect", errors.New("--kind and --id are required"))
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.PermissionInspector.InspectAdminPermission(cmd.Context(), in)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&in.ResourceKind, "kind", "", "exact resource kind: workbook, datasource, flow, or project")
	cmd.Flags().StringVar(&in.ResourceLUID, "id", "", "authoritative resource LUID")
	cmd.Flags().StringVar(&in.DefaultFor, "default-for", "", "project default content kind: workbooks, datasources, or flows")
	cmd.Flags().StringVar(&in.PrincipalType, "principal-type", "", "exact principal type: user or group")
	cmd.Flags().StringVar(&in.PrincipalLUID, "principal-id", "", "authoritative principal LUID filter")
	cmd.Flags().StringVar(&in.Capability, "capability", "", "exact capability-name filter")
	return cmd
}

func mutation(use, short, capability string, _ bool, args cobra.PositionalArgs, run func(*cobra.Command) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Annotations: map[string]string{"tadx.capability": capability}, Args: args, RunE: func(cmd *cobra.Command, _ []string) error { return run(cmd) }}
}
func noArgs(operation string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage(operation, err)
		}
		return nil
	}
}
func requireMutation(_ *cobra.Command, operation, environment string) error {
	if environment == "" {
		return clierr.Usage(operation, errors.New("--environment is required for remote mutation"))
	}
	return nil
}
func setString(cmd *cobra.Command, name, value string, target **string) {
	if cmd.Flags().Changed(name) {
		copy := value
		*target = &copy
	}
}
