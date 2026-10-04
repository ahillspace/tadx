package admin

import (
	"errors"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newGroupMemberAdd(deps Dependencies) *cobra.Command {
	var in groupops.MembershipInput
	var preview bool
	cmd := mutation("add", "Add one user to one group.", "admin.group.member.add", func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.member.add")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.group.member.add", in.Environment); err != nil {
			return err
		}
		if in.GroupLUID == "" || (in.UserLUID == "") == (in.Username == "") {
			return clierr.Usage("admin.group.member.add", errors.New("--group-id and exactly one of --user-id or --username are required"))
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.GroupMemberAdder.AddAdminGroupMember(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "group-id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&in.UserLUID, "user-id", "", "authoritative user LUID")
	cmd.Flags().StringVar(&in.Username, "username", "", "exact site username, never a display name; mutually exclusive with --user-id")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}

func newGroupMemberRemove(deps Dependencies) *cobra.Command {
	var in groupops.MembershipInput
	var preview bool
	cmd := mutation("remove", "Remove one user from one group.", "admin.group.member.remove", func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.member.remove")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.group.member.remove", in.Environment); err != nil {
			return err
		}
		if in.GroupLUID == "" || (in.UserLUID == "") == (in.Username == "") {
			return clierr.Usage("admin.group.member.remove", errors.New("--group-id and exactly one of --user-id or --username are required"))
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.GroupMemberRemover.RemoveAdminGroupMember(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "group-id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&in.UserLUID, "user-id", "", "authoritative user LUID")
	cmd.Flags().StringVar(&in.Username, "username", "", "exact site username, never a display name; mutually exclusive with --user-id")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
