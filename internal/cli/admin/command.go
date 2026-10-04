// Package admin contains thin Cobra plumbing for Tableau administration commands.
package admin

import (
	"context"
	"errors"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	permission "github.com/ahillspace/tadx/actions/admin/permission"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type Renderer interface{ Render(any) error }
type UserLister interface {
	ListAdminUsers(context.Context, userops.ListInput) (userops.ListOutput, error)
}
type UserInspector interface {
	InspectAdminUser(context.Context, userops.InspectInput) (userops.InspectOutput, error)
}
type UserCreator interface {
	CreateAdminUser(context.Context, userops.CreateInput, bool) (userops.CreateOutput, error)
}
type UserUpdater interface {
	UpdateAdminUser(context.Context, userops.UpdateInput, bool) (userops.UpdateOutput, error)
}
type UserDeleter interface {
	DeleteAdminUser(context.Context, userops.DeleteInput, bool) (userops.DeleteOutput, error)
}
type GroupLister interface {
	ListAdminGroups(context.Context, groupops.ListInput) (groupops.ListOutput, error)
}
type GroupInspector interface {
	InspectAdminGroup(context.Context, groupops.InspectInput) (groupops.InspectOutput, error)
}
type GroupCreator interface {
	CreateAdminGroup(context.Context, groupops.CreateInput, bool) (groupops.CreateOutput, error)
}
type GroupUpdater interface {
	UpdateAdminGroup(context.Context, groupops.UpdateInput, bool) (groupops.UpdateOutput, error)
}
type GroupDeleter interface {
	DeleteAdminGroup(context.Context, groupops.DeleteInput, bool) (groupops.DeleteOutput, error)
}
type GroupMemberAdder interface {
	AddAdminGroupMember(context.Context, groupops.MembershipInput, bool) (groupops.MembershipOutput, error)
}
type GroupMemberRemover interface {
	RemoveAdminGroupMember(context.Context, groupops.MembershipInput, bool) (groupops.MembershipOutput, error)
}
type PermissionInspector interface {
	InspectAdminPermission(context.Context, permission.InspectInput) (permission.InspectOutput, error)
}

type Dependencies struct {
	PermissionCapabilities func(string) []string
	PermissionCreator      PermissionCreator
	PermissionDeleter      PermissionDeleter
	UserLister             UserLister
	UserInspector          UserInspector
	UserCreator            UserCreator
	UserUpdater            UserUpdater
	UserDeleter            UserDeleter
	GroupLister            GroupLister
	GroupInspector         GroupInspector
	GroupCreator           GroupCreator
	GroupUpdater           GroupUpdater
	GroupDeleter           GroupDeleter
	GroupMemberAdder       GroupMemberAdder
	GroupMemberRemover     GroupMemberRemover
	PermissionInspector    PermissionInspector
	Renderer               Renderer
}

func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "admin", Short: "Administer Tableau users, groups, and permissions"}
	user := &cobra.Command{Use: "user", Short: "Administer site users"}
	user.AddCommand(newUserList(deps), newUserInspect(deps), newUserCreate(deps), newUserUpdate(deps), newUserDelete(deps))
	group := &cobra.Command{Use: "group", Short: "Administer site groups"}
	group.AddCommand(newGroupList(deps), newGroupInspect(deps), newGroupCreate(deps), newGroupUpdate(deps), newGroupDelete(deps))
	member := &cobra.Command{Use: "group-member", Short: "Change direct group membership"}
	member.AddCommand(newGroupMemberAdd(deps), newGroupMemberRemove(deps))
	permission := &cobra.Command{Use: "permission", Short: "Manage exact permission rules"}
	permission.AddCommand(newPermissionInspect(deps), newPermissionCreate(deps), newPermissionDelete(deps))
	command.AddCommand(user, group, member, permission)
	return command
}

func mutation(use, short, capability string, args cobra.PositionalArgs, run func(*cobra.Command) error) *cobra.Command {
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
