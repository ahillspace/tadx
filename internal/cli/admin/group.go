package admin

import (
	"errors"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newGroupList(deps Dependencies) *cobra.Command {
	var in groupops.ListInput
	cmd := &cobra.Command{Use: "list", Short: "List groups with bounded live reads or explicit --all.", Annotations: map[string]string{"tadx.capability": "admin.group.list"}, Args: noArgs("admin.group.list"), RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.GroupLister.ListAdminGroups(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact group-name filter")
	cmd.Flags().StringVar(&in.Domain, "domain", "", "exact directory-domain filter")
	cmd.Flags().BoolVar(&in.All, "all", false, "return all matching records, up to 10000; cannot combine with --limit")
	cmd.Flags().IntVar(&in.Limit, "limit", 0, "maximum groups to render, up to 10000")
	cmd.Flags().StringVar(&in.Cursor, "cursor", "", "opaque continuation cursor")
	cmd.MarkFlagsMutuallyExclusive("all", "limit")
	cmd.MarkFlagsMutuallyExclusive("all", "cursor")
	cmd.Flags().BoolVar(&in.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return cmd
}

func newGroupInspect(deps Dependencies) *cobra.Command {
	var in groupops.InspectInput
	var id, name string
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect one exact group.", Annotations: map[string]string{"tadx.capability": "admin.group.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.group.inspect")(cmd, args); err != nil {
			return err
		}
		if (id == "") == (name == "") {
			return clierr.Usage("admin.group.inspect", errors.New("use exactly one of --id or --name"))
		}
		in.Selector = groupops.Selector{LUID: id, Name: name}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.GroupInspector.InspectAdminGroup(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&id, "id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&name, "name", "", "exact group name")
	cmd.Flags().BoolVar(&in.IncludeMembers, "members", false, "include all observed direct members")
	cmd.Flags().BoolVar(&in.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return cmd
}

func newGroupCreate(deps Dependencies) *cobra.Command {
	var in groupops.CreateInput
	var external bool
	var preview bool
	cmd := mutation("create", "Create one exact group.", "admin.group.create", func(cmd *cobra.Command, args []string) error {
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
			return clierr.WithOutput(out, err)
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
	var in groupops.UpdateInput
	var name, role string
	var external, setMembers, preview bool
	var members []string
	cmd := mutation("update", "Update one exact group.", "admin.group.update", func(cmd *cobra.Command, args []string) error {
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
		setString(cmd, "new-name", name, &in.Name)
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
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "id", "", "authoritative group LUID")
	cmd.Flags().StringVar(&name, "new-name", "", "explicit new group name")
	cmd.Flags().StringVar(&name, "name", "", "compatibility alias for --new-name")
	_ = cmd.Flags().MarkHidden("name")
	cmd.MarkFlagsMutuallyExclusive("name", "new-name")
	cmd.Flags().StringVar(&role, "minimum-site-role", "", "explicit minimum site role")
	cmd.Flags().BoolVar(&external, "external-user-enabled", false, "explicit on-demand external-user setting")
	cmd.Flags().BoolVar(&setMembers, "set-members", false, "converge direct membership to the repeated --member-id values, including an empty set")
	cmd.Flags().StringArrayVar(&members, "member-id", nil, "authoritative desired direct-member LUID; repeat for each member")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}

func newGroupDelete(deps Dependencies) *cobra.Command {
	var in groupops.DeleteInput
	var preview bool
	cmd := mutation("delete", "Delete one exact group.", "admin.group.delete", func(cmd *cobra.Command, args []string) error {
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
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.GroupLUID, "id", "", "authoritative group LUID")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
